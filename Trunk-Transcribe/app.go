package main

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/google/generative-ai-go/genai"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/slack-go/slack"
	"golang.org/x/sync/errgroup"
	"google.golang.org/api/option"
)

type SlackChannelID string

const (
	UCPD       SlackChannelID = "C06J8T3EUP9"
	BERKELEY   SlackChannelID = "C06A28PMXFZ"
	OAKLAND                   = "C070R7LGVDY"
	ALBANY                    = "C0713T4KMMX"
	EMERYVILLE                = "C07123TKG3E"
)

type SlackUserID string

const (
	EMILIE  SlackUserID = "U06H9NA2L4V"
	MARC                = "U03FTUS9SSD"
	NAVEEN              = "U0531U1RY1W"
	JOSE                = "U073Q372CP9"
	STEPHAN             = "U06UWE5EDAT"
	HELEN               = "U08155VNVRQ"
)

var (
	puncRegex    = regexp.MustCompile("[\\.\\!\\?;]\\s+")
	wordsRegex   = regexp.MustCompile("[a-zA-Z0-9_-]+")
	numericRegex = regexp.MustCompile("[0-9]+")
	modeString   = `(auto|car|driver|vehicle|bike|pedestrian|ped|bicycle|cyclist|bicyclist|pavement)s?`
	versusRegex  = regexp.MustCompile(modeString + `.+(vs|versus|verses)(\.)?.+` + modeString)
	streets      = []string{"Acton", "Ada", "Addison", "Adeline", "Alcatraz", "Allston", "Ashby", "Bancroft", "Benvenue", "Berryman", "Blake", "Bonar", "Bonita", "Bowditch", "Buena", "California", "Camelia", "Carleton", "Carlotta", "Cedar", "Center", "Channing", "Chestnut", "Claremont", "Codornices", "College", "Cragmont", "Delaware", "Derby", "Dwight", "Eastshore", "Edith", "Elmwood", "Euclid", "Francisco", "Fresno", "Gilman", "Grizzly", "Harrison", "Hearst", "Heinz", "Henry", "Hillegass", "Holly", "Hopkins", "Josephine", "Kains", "King", "LeConte", "LeRoy", "Hilgard", "Mabel", "Marin", "Martin", "MLK", "Milvia", "Monterey", "Napa", "Neilson", "Oregon", "Parker", "Piedmont", "Posen", "Rose", "Russell", "Sacramento", "San Pablo", "Santa", "Fe", "Shattuck", "Solano", "Sonoma", "Spruce", "Telegraph", "Alameda", "Thousand", "Oaks", "University", "Vine", "Virginia", "Ward", "Woolsey"}
	modifiers    = []string{"street", "boulevard", "road", "path", "way", "avenue", "highway"}
	terms        = []string{"bike", "bicycle", "pedestrian", "vehicle", "injury", "victim", "versus", "transport", "concious", "breathing", "alta bates", "highland", "BFD", "Adam", "ID tech", "ring on three", "code 2", "code 3", "code 4", "code 34", "en route", "case number", "berry brothers", "rita run", "DBF", "Flock camera", "10-four", "10-4", "10 four", "His Lordships", "Cesar Chavez Park", "10-9 your traffic", "copy", "tow", "kill the beeper"}

	defaultChannelID = BERKELEY // #scanner-dispatches

	talkgroupToChannel = map[int64]SlackChannelID{
		3605: UCPD, // UCB PD1 : #scanner-dispatches-ucpd
		3606: UCPD, // UCB PD2 : #scanner-dispatches-ucpd
		3608: UCPD, // UCB PD4 : #scanner-dispatches-ucpd
		3609: UCPD, // UCB PD5 : #scanner-dispatches-ucpd
		3055: ALBANY,
		3056: ALBANY,
		3057: ALBANY,
		3058: ALBANY,
		3059: ALBANY,
		2050: ALBANY,
		2055: ALBANY,
		2056: ALBANY,
		2057: ALBANY,
		2058: ALBANY,
		2059: ALBANY,
		4055: ALBANY,
		3155: EMERYVILLE,
		3156: EMERYVILLE,
		3157: EMERYVILLE,
		4155: EMERYVILLE,
		3405: OAKLAND,
		3406: OAKLAND,
		3407: OAKLAND,
		3408: OAKLAND,
		3409: OAKLAND,
		3410: OAKLAND,
		3411: OAKLAND,
		3418: OAKLAND,
		3419: OAKLAND,
		3420: OAKLAND,
		3421: OAKLAND,
		3422: OAKLAND,
		3423: OAKLAND,
		3424: OAKLAND,
		3425: OAKLAND,
		3426: OAKLAND,
		3428: OAKLAND,
		3429: OAKLAND,
		3447: OAKLAND,
		3448: OAKLAND,
		2400: OAKLAND,
		2405: OAKLAND,
		2406: OAKLAND,
		2407: OAKLAND,
		2408: OAKLAND,
		2409: OAKLAND,
		2410: OAKLAND,
		2411: OAKLAND,
		2412: OAKLAND,
		2413: OAKLAND,
		2414: OAKLAND,
		2416: OAKLAND,
		2417: OAKLAND,
		2434: OAKLAND,
		2436: OAKLAND,
		4405: OAKLAND,
		4407: OAKLAND,
		4415: OAKLAND,
		4421: OAKLAND,
		4422: OAKLAND,
		4423: OAKLAND,
	}

	location *time.Location
)

//slack user id to keywords map
// supports regex

var notifsMap = map[SlackUserID]Notifs{
	EMILIE: Notifs{
		Include:  []string{"auto ped", "auto-ped", "autoped", "autobike", "auto bike", "auto bicycle", "auto-bike", "auto-bicycle", "hit and run", "1071", "GSW", "loud reports", "211", "highland", "catalytic", "apple", "261", "code 3", "10-15", "beeper", "1053", "1054", "1055", "1080", "1199", "DBF", "Code 33", "1180", "215", "220", "243", "244", "243", "288", "451", "288A", "243", "207", "212.5", "1079", "1067", "accident", "collision", "fled", "homicide", "fait", "fate", "injuries", "conscious", "responsive", "shooting", "shoot", "coroner", "weapon", "weapons", "gun", "flock", "spikes", "challenging", "beeper", "cage", "tom", "register", "1033 frank", "1033f", "1033", "10-33 frank", "pursuit", "frank"},
		NotRegex: regexp.MustCompile("no (weapon|gun)s?"),
		Regex:    versusRegex,
		Channels: []SlackChannelID{BERKELEY, UCPD},
	},
	NAVEEN: Notifs{
		Include:  []string{"hit and run", "auto ped", "auto-ped", "autoped", "autobike", "auto bicycle", "auto-bike", "auto-bicycle", "Rose St", "Rose Street", "Ruth Acty", "King Middle"},
		Regex:    versusRegex,
		Channels: []SlackChannelID{BERKELEY, UCPD, ALBANY, EMERYVILLE},
	},
	MARC: Notifs{
		Include:  []string{"hit and run", "autobike", "auto bike", "auto bicycle", "auto bicyclist", "auto ped", "auto-ped", "autoped"},
		Regex:    versusRegex,
		Channels: []SlackChannelID{BERKELEY, UCPD},
	},
	JOSE: Notifs{
		Include:  []string{"accident", "collision", "crash", "crashed", "crashes"},
		Regex:    versusRegex,
		Channels: []SlackChannelID{OAKLAND},
	},
	STEPHAN: Notifs{
		Include:  []string{"GSW", "Active Shooter", "Shots Fired", "Pursuit", "Structure Fire", "Shooting", "Shooter", "Shots", "Code 33", "glock"},
		NotRegex: regexp.MustCompile("no (weapon|gun)s?"),
		Channels: []SlackChannelID{BERKELEY, UCPD},
	},
	HELEN: Notifs{
		Include:  []string{"hit and run", "autobike", "auto bike", "auto bicycle", "auto bicyclist", "auto ped", "auto-ped", "autoped", "marin", "hopkins", "el dorado"},
		Regex:    versusRegex,
		Channels: []SlackChannelID{BERKELEY},
	},
}

var r2Key string = os.Getenv("CLOUDFLARE_R2_KEY")
var r2Secret string = os.Getenv("CLOUDFLARE_R2_SECRET")

// google gemini
var geminiApiKey string = os.Getenv("GEMINI_API_KEY")

// slack setup
var webhookUrl string = os.Getenv("SLACK_WEBHOOK_URL")
var webhookUrlUCPD string = os.Getenv("SLACK_WEBHOOK_URL_UCPD")
var slackapiSecret string = os.Getenv("SLACK_API_SECRET")

//go:embed templates/*
var resources embed.FS

var t = template.Must(template.ParseFS(resources, "templates/*"))

type Config struct {
	uploader       *s3manager.Uploader
	slackClient    *slack.Client
	webhookUrl     string
	webhookUrlUCPD string
}

var dedupeCache *lru.Cache[string, bool]

func init() {
	var err error
	location, err = time.LoadLocation("America/Los_Angeles")
	if err != nil {
		panic(err)
	}

	dedupeCache, err = lru.New[string, bool](1000)
	if err != nil {
		panic(err)
	}
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"

	}

	var api *slack.Client
	if slackapiSecret == "" || webhookUrl == "" {
		log.Println("Missing SLACK_API_SECRET or SLACK_WEBHOOK_URL. Slack notifications disabled.")
	} else {
		api = slack.New(slackapiSecret)
	}

	// R2 setup
	endpoint := fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cloudflareAccountID)
	fmt.Println("Using cloudflare R2 endpoint: ", endpoint)

	r2Config := &aws.Config{
		Region:      aws.String("auto"),
		Credentials: credentials.NewStaticCredentials(r2Key, r2Secret, ""),
		Endpoint:    aws.String(endpoint),
	}
	uploader := s3manager.NewUploader(session.New(r2Config))

	config := &Config{
		uploader:       uploader,
		slackClient:    api,
		webhookUrl:     webhookUrl,
		webhookUrlUCPD: webhookUrlUCPD,
	}

	ch := make(chan *TranscriptionRequest)
	var wg sync.WaitGroup

	// start transcription request goroutine pool
	go func() {

		for req := range ch {
			var requestsInFlight atomic.Int64
			wg.Add(1)
			requestsInFlight.Add(1)
			fmt.Println("Requests in flight: ", requestsInFlight.Load())
			go func() {
				ctx := context.Background()
				handleTranscriptionRequest(ctx, config, req)
				wg.Done()
				requestsInFlight.Add(-1)
			}()
		}
	}()

	// create server to serve http requests
	server := &http.Server{
		Addr:    ":8080",
		Handler: mux(config, ch),
	}

	log.Println("Starting server on port: ", port)

	// start server
	go func() {
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server error: %v", err)
		}
		log.Println("Stopped serving new connections.")
	}()

	// handle sigint
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	shutdownCtx, shutdownRelease := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownRelease()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("HTTP shutdown error: %v", err)
	}

	// close TranscriptionRequest channel
	close(ch)

	// wait for transcription request go routines to complete
	wg.Wait()

	log.Println("Graceful shutdown complete.")
}

// mux creates a new ServeMux router
func mux(config *Config, ch chan *TranscriptionRequest) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data := map[string]string{
			"Region": os.Getenv("FLY_REGION"),
		}
		t.ExecuteTemplate(w, "index.html.tmpl", data)
	})

	mux.HandleFunc("/audio", func(w http.ResponseWriter, r *http.Request) {
		link := r.URL.Query()["link"]
		if len(link) < 1 {
			writeErr(w, errors.New("link not provided"))
			return
		}

		result, err := url.JoinPath("https://pub-85c4b9a9667540e99c0109c068c47e0f.r2.dev", link[0])
		if err != nil {
			writeErr(w, err)
			return
		}
		data := map[string]string{
			"Link": result,
		}
		t.ExecuteTemplate(w, "audio.html.tmpl", data)
	})

	mux.HandleFunc("/transcribe", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 20<<20+512)
		req, err := createTransciptionRequest(r.Context(), config, r)
		if err != nil {
			writeErr(w, err)
			return
		}
		ch <- req

		http.ServeContent(w, r, "", time.Now(), strings.NewReader("ok"))
	})

	return mux
}

// createTransciptionRequest creates a TranscriptionRequest object from the incoming http request
func createTransciptionRequest(ctx context.Context, config *Config, r *http.Request) (*TranscriptionRequest, error) {

	err := r.ParseMultipartForm(1 << 20)
	if err != nil {
		return nil, err
	}
	js, _, err := r.FormFile("call_json")
	if err != nil {
		return nil, err
	}
	defer js.Close()
	callJson, _ := ioutil.ReadAll(js)

	file, header, err := r.FormFile("call_audio")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	filename := filepath.Base(header.Filename)
	data, _ := ioutil.ReadAll(file)

	return NewTranscriptionRequest(filename, data, callJson)
}

// handleTranscriptionRequest
func handleTranscriptionRequest(ctx context.Context, config *Config, req *TranscriptionRequest) error {
	var err error
	start := time.Now()
	log.Println("Handling transcription request: ", req.Filename)

	defer func() {
		duration := time.Now().Sub(start)
		if err != nil {
			log.Printf("Faileds transcription request [%v]: %s : %v", duration, req.Filename, err)
		} else {
			log.Printf("Finished transcription request [%v]: %s", duration, req.Filename)
		}
	}()

	enhanceCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	enhanced, enhanceErr := io.ReadAll(removeSilence(enhanceCtx, req.Data))
	// enhanced, enhanceErr := deepFilter(enhanceCtx, req.Data)

	if enhanceErr != nil {
		log.Println("[handleTranscriptionRequest] Error performing enhancement on audio. Falling back to original. ", enhanceErr)
	} else {
		req.Data = enhanced
	}

	var wg sync.WaitGroup
	go func() {
		wg.Add(1)
		_, err = transcribeAndUpload(ctx, config, req)
		wg.Done()
		if err != nil {
			fmt.Println("[handleTranscriptionRequest]Error transcribing and uploading to slack: ", err.Error())
		}
	}()

	go func() {
		wg.Add(1)
		err = uploadToRdio(ctx, req)
		wg.Done()
		if err != nil {
			fmt.Println("[handleTranscriptionRequest] Error uploading to rdio: ", err.Error())
		}
	}()

	wg.Wait()
	return nil
}

// dedupeDispatch checks if the specified dispatch (described by its metadata) has already been seen.
// Returns true if the dispatch is a duplicate, false otherwise.
func dedupeDispatch(meta Metadata) (dupe bool) {

	// construct a cache key consisting of all the srcs (parties in the call),
	// the talkgroup, and the startime
	var srcs string
	for _, src := range meta.SrcList {
		srcs += fmt.Sprintf(".%v", src.Src)
	}

	startTime := meta.StartTime - (meta.StartTime % 5) // time to the nearest 5 second increment
	dedupeKey := fmt.Sprintf("tg.%d.start.%d.srcs%s", meta.Talkgroup, startTime, srcs)

	// atomically check-or-set. Return whether the key already existed.
	exists, _ := dedupeCache.ContainsOrAdd(dedupeKey, true)

	return exists
}

// transcribeAndUpload transcribes the audio to text, posts the text to slack and persists the audio file to S3,
func transcribeAndUpload(ctx context.Context, config *Config, req *TranscriptionRequest) (string, error) {

	key := req.FilePath()
	data := req.Data
	metadata := req.Meta

	msg, segments, err := whisper(ctx, req.Data)

	if err == nil {
		fmt.Println(key+": ", msg)
	} else {
		msg = "Error transcribing text: " + err.Error()
	}

	metadata.AudioText = msg
	metadata.Segments = segments
	metadata.URL = fmt.Sprintf("https://trunk-transcribe.fly.dev/audio?link=%s", key)

	wg, gctx := errgroup.WithContext(ctx)

	//upload to Cloudflare R2 (with s3 compatible api)
	wg.Go(func() error {
		return uploadS3(gctx, config.uploader, key, bytes.NewReader(data), metadata)
	})

	wg.Go(func() error {
		return postToSlack(gctx, config, key, data, metadata)
	})
	err = wg.Wait()
	return msg, err
}

func gemini(ctx context.Context, data []byte) (string, error) {

	client, err := genai.NewClient(ctx, option.WithAPIKey(geminiApiKey))
	if err != nil {
		return "", err
	}
	defer client.Close()

	prompt := strings.Join(append(streets, append(modifiers, terms...)...), ", ")
	parts := []genai.Part{
		genai.Blob{MIMEType: "audio/mp3", Data: data},
		genai.Text("Please transcribe the audio. "),
		genai.Text("Ignore silences."),
		genai.Text("Here are some correction terms: " + prompt),
	}

	model := client.GenerativeModel("gemini-1.5-pro")
	resp, err := model.GenerateContent(ctx, parts...)
	if err != nil {
		return "", err
	}

	var transcriptionParts []string
	for _, c := range resp.Candidates {
		if c.Content == nil {
			continue
		}
		for _, part := range c.Content.Parts {
			line := fmt.Sprintf("%v", part)
			if strings.TrimSpace(line) == "" {
				continue
			}
			transcriptionParts = append(transcriptionParts, line)
		}
	}

	msg := strings.Join(transcriptionParts, "\n")
	return msg, nil
}

// transcribeAndUpload uploads the audio to S3
func uploadS3(ctx context.Context, uploader *s3manager.Uploader, key string, reader io.Reader, meta Metadata) error {

	s3Meta := make(map[string]*string)
	s3Meta["short-name"] = aws.String(meta.ShortName)
	s3Meta["call-length"] = aws.String(strconv.FormatInt(meta.CallLength, 10))
	s3Meta["talk-group"] = aws.String(strconv.FormatInt(meta.Talkgroup, 10))
	s3Meta["priority"] = aws.String(strconv.FormatInt(meta.Priority, 10))

	input := &s3manager.UploadInput{
		Bucket:      aws.String("scanner-berkeley"),         // bucket's name
		Key:         aws.String(key),                        // files destination location
		Body:        reader,                                 // content of the file
		Metadata:    s3Meta,                                 // metadata
		ContentType: aws.String("application/octet-stream"), // content type
	}
	_, err := uploader.UploadWithContext(ctx, input)
	return err
}

func postToSlack(ctx context.Context, config *Config, key string, data []byte, meta Metadata) error {
	reader := bytes.NewReader(data)

	if config.slackClient == nil {
		log.Println("Missing SLACK_API_SECRET or SLACK_WEBHOOK_URL. Slack notifications disabled.")
		return nil
	}
	if meta.AudioText == "" {
		meta.AudioText = "Could not transcribe audio"
	}

	blocks := meta.Segments

	for i, block := range blocks {
		if i >= len(meta.SrcList) {
			break
		} else if len(block) == 0 {
			continue
		}

		src := meta.SrcList[i]
		tag := src.Tag
		if tag == "" {
			tag = strconv.FormatInt(src.Src, 10)
		}
		block = strings.TrimSpace(block)
		blocks[i] = tag + ": " + block
	}

	// determine channel
	channelID, ok := talkgroupToChannel[meta.Talkgroup]
	if !ok {
		channelID = defaultChannelID
	}

	slackMeta := ExtractSlackMeta(meta, channelID, notifsMap)
	mentions := slackMeta.Mentions
	if str := strings.Join(mentions, " "); len(str) > 0 {
		blocks = append(blocks, str)
	}
	blocks = append([]string{"*" + meta.TalkgroupTag + "* | _" + meta.TalkGroupDesc + "_"}, blocks...)
	blocks = append(blocks, fmt.Sprintf("<%s|Audio>", meta.URL))
	if addr := slackMeta.Address.String(); len(addr) > 0 {
		blocks = append(blocks, "Location: "+addr)
	}

	blocks = append(blocks, fmt.Sprintf("%d seconds | %s", meta.CallLength, time.Now().In(location).Format("Mon, Jan 02 2006 3:04PM MST")))
	sentances := strings.Join(blocks, "\n")

	// upload audio
	filename := filepath.Base(key)
	_, err := config.slackClient.UploadFileV2Context(ctx, slack.UploadFileV2Parameters{
		Filename:       filename,
		FileSize:       len(data),
		Reader:         reader,
		InitialComment: sentances,
		Channel:        string(channelID),
	})
	if err != nil {
		log.Println("Error uploading file to slack: ", err)
	} /*else {
		log.Printf("Uploaded file: %s with title: %s to channel: %v", summary.ID, summary.Title, channelID)
	}*/

	return err
}

// uploadToRdio uploads the audio file to the radio interface: https://rdio-eastbay.fly.dev
func uploadToRdio(ctx context.Context, req *TranscriptionRequest) error {

	filename, meta, reader := req.Filename, req.MetaRaw, bytes.NewReader(req.Data)

	rdioScannerSecret := os.Getenv("RDIO_SCANNER_API_KEY")
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	defer writer.Close()

	writer.WriteField("key", rdioScannerSecret)
	writer.WriteField("meta", string(meta))
	writer.WriteField("system", "1000") // "eastbay" system
	part, _ := writer.CreateFormFile("audio", filename)

	io.Copy(part, reader)
	writer.Close()

	uri := "https://rdio-eastbay.fly.dev/api/trunk-recorder-call-upload"
	res, err := http.Post(uri, writer.FormDataContentType(), body)
	if err != nil {
		return err
	}
	resBody, _ := io.ReadAll(res.Body)
	defer res.Body.Close()

	if res.StatusCode > 299 {
		return fmt.Errorf("Error uploading to rdio-scanner: Response failed with status code: %d and\nbody: %s\n", res.StatusCode, resBody)
	}
	return nil
}

// ExtractSlackMeta returns the list of mentions and an address to append corresponding to matching keywords in the
// sentance
// It accepts a sentace to match keywords against. The keywords map provides a map
// of mentions to keywords to match.

func ExtractSlackMeta(meta Metadata, channelID SlackChannelID, notifsMap map[SlackUserID]Notifs) (slackMeta SlackMeta) {

	text := strings.ToLower(meta.AudioText)
	words := wordsRegex.FindAllString(text, -1) //split text into words array

	for userID, notifs := range notifsMap {

		switch {
		case !slices.Contains(notifs.Channels, channelID): // user not listening to channel
			continue
		case notifs.NotRegex != nil && notifs.NotRegex.MatchString(text): // notregex matches text. skip
			continue
		case notifs.Regex != nil && notifs.Regex.MatchString(text): // regex matches text. append
			slackMeta.Mentions = append(slackMeta.Mentions, "<@"+string(userID)+">")
			continue
		}

		// check the included keywords
	match:
		for _, keyword := range notifs.Include {
			keyword = strings.ToLower(keyword)
			sequence := wordsRegex.FindAllString(keyword, -1)
			for chunk := range slices.Chunk(words, len(sequence)) {
				if slices.Equal(chunk, sequence) {
					slackMeta.Mentions = append(slackMeta.Mentions, "<@"+string(userID)+">")
					break match
				}
			}
		}
	}

	// match address
	for i := 0; i < len(words); i += 1 {
		prefix := words[i]
		var word string
		if len(words[i:]) >= 2 {
			word = words[i+1]
		}

		hasAddrNumber := numericRegex.MatchString(prefix)

		for _, street := range streets {
			lowerCase := strings.ToLower(street)
			found := false
			switch {
			case hasAddrNumber && word == lowerCase:
				slackMeta.Address.PrimaryAddress = prefix + " " + street
				// incr index by one to move on to the next pair
				i += 1
				found = true
			case prefix == lowerCase:
				slackMeta.Address = slackMeta.Address.AppendStreet(street)
				found = true
			}
			if found {
				break
			}
		}

	}

	return slackMeta
}

func writeErr(w http.ResponseWriter, err error) {
	fmt.Println("Error: ", err.Error())
	http.Error(w, err.Error(), http.StatusInternalServerError)
}
