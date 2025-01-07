package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
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
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	lru "github.com/hashicorp/golang-lru/v2"	
	"github.com/slack-go/slack"
	"golang.org/x/sync/errgroup"
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
	terms        = []string{"bike", "bicycle", "pedestrian", "vehicle", "injury", "victim", "versus", "transport", "concious", "breathing", "alta bates", "highland", "BFD", "Adam", "ID tech", "ring on three", "code 4", "code 34", "en route", "case number", "berry brothers", "rita run", "DBF", "Flock camera", "10-four", "10-4", "10 four", "His Lordships", "Cesar Chavez Park", "10-9 your traffic", "copy", "tow"}

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
		Include:  []string{"auto ped", "auto-ped", "autoped","autobike", "auto bike", "auto bicycle", "auto-bike", "auto-bicycle", "hit and run", "1071", "GSW", "loud reports", "211", "highland", "catalytic", "apple", "261", "code 3", "10-15", "beeper", "1053", "1054", "1055", "1080", "1199", "DBF", "Code 33", "1180", "215", "220", "243", "244", "243", "288", "451", "288A", "243", "207", "212.5", "1079", "1067", "accident", "collision", "fled", "homicide", "fait", "fate", "injuries", "conscious", "responsive", "shooting", "shoot", "coroner", "weapon", "weapons", "gun", "flock", "spikes", "challenging", "beeper", "cage", "tom", "register", "1033 frank", "1033f", "1033", "10-33 frank"},
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
		Include: []string{"hit and run", "autobike", "auto bike", "auto bicycle", "auto bicyclist", "auto ped", "auto-ped", "autoped"},
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
		Include: []string{"hit and run", "autobike", "auto bike", "auto bicycle", "auto bicyclist", "auto ped", "auto-ped", "autoped", "marin", "hopkins"},
		Regex:    versusRegex,
		Channels: []SlackChannelID{BERKELEY},
	},
}

// cloudflare setup
var cloudflareApiToken string = os.Getenv("CLOUDFLARE_API_TOKEN")
var cloudflareAccountID string = os.Getenv("CLOUDFLARE_ACCOUNT_ID")
var cloudflareWhisperUrl string = "https://api.cloudflare.com/client/v4/accounts/"+cloudflareAccountID+"/ai/run/@cf/openai/whisper-large-v3-turbo"
var r2Key string = os.Getenv("CLOUDFLARE_R2_KEY")
var r2Secret string = os.Getenv("CLOUDFLARE_R2_SECRET")

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
		Endpoint: aws.String(endpoint),		
	}	
	uploader := s3manager.NewUploader(session.New(r2Config))
	
	config := &Config{		
		uploader:       uploader,		
		slackClient:    api,
		webhookUrl:     webhookUrl,
		webhookUrlUCPD: webhookUrlUCPD,
	}

	mux := NewMux(config)

	log.Println("listening on", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

// NewMux creates a new ServeMux router
func NewMux(config *Config) *http.ServeMux {
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
		err := handleTranscription(r.Context(), config, r)
		if err != nil {
			writeErr(w, err)
			return
		}
		http.ServeContent(w, r, "", time.Now(), strings.NewReader("ok"))
	})

	return mux
}

func handleTranscription(ctx context.Context, config *Config, r *http.Request) error {

	err := r.ParseMultipartForm(1 << 20)
	if err != nil {
		return err
	}
	js, _, err := r.FormFile("call_json")
	if err != nil {
		return err
	}
	defer js.Close()
	callJson, _ := ioutil.ReadAll(js)

	file, header, err := r.FormFile("call_audio")
	if err != nil {
		return err
	}
	defer file.Close()

	filename := filepath.Base(header.Filename)
	data, _ := ioutil.ReadAll(file)

	var meta Metadata
	err = json.Unmarshal(callJson, &meta)
	if err != nil {
		return err
	}

	if dedupeDispatch(meta) {
		log.Printf("Ignoring duplicate dispatch: %d, \n%s\n", meta.Talkgroup, meta.AudioText)
		return nil
	}

	// fire goroutine and return to unblock client resources
	go func() {
		filepath := fmt.Sprintf("%s/%d/%s", meta.ShortName, meta.Talkgroup, filename)
		_, err := transcribeAndUpload(context.Background(), config, filepath, data, meta)
		if err != nil {
			fmt.Println("Error transcribing and uploading to slack: ", err.Error())
		}
	}()

	// upload to rdio
	go func() {
		rdioScannerSecret := os.Getenv("RDIO_SCANNER_API_KEY")
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		defer writer.Close()

		writer.WriteField("key", rdioScannerSecret)
		writer.WriteField("meta", string(callJson))
		writer.WriteField("system", "1000") // "eastbay" system
		part, _ := writer.CreateFormFile("audio", filename)

		io.Copy(part, bytes.NewBuffer(data))
		writer.Close()

		uri := "https://rdio-eastbay.fly.dev/api/trunk-recorder-call-upload"
		res, err := http.Post(uri, writer.FormDataContentType(), body)
		if err != nil {
			fmt.Println("Error uploading to rdio-scanner: ", err.Error())
			return
		}
		resBody, _ := io.ReadAll(res.Body)
		defer res.Body.Close()

		if res.StatusCode > 299 {
			fmt.Printf("Error uploading to rdio-scanner: Response failed with status code: %d and\nbody: %s\n", res.StatusCode, resBody)
		}
	}()

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
func transcribeAndUpload(ctx context.Context, config *Config, key string, data []byte, metadata Metadata) (string, error) {

<<<<<<< Updated upstream
	msg, err := whisper(ctx, data)
=======
	// msg, err := whisper(ctx, data)
	msg, err := gemini(ctx, data)

>>>>>>> Stashed changes
	if err == nil {
		fmt.Println(key+": ", msg)
	} else {
		msg = "Error transcribing text: " + err.Error()
	}

	metadata.AudioText = msg
	metadata.URL = fmt.Sprintf("https://trunk-transcribe.fly.dev/audio?link=%s", key)

<<<<<<< Updated upstream
	wg, gctx := errgroup.WithContext(ctx)	
=======
	wg, gctx := errgroup.WithContext(ctx)
>>>>>>> Stashed changes

	//upload to Cloudflare R2 (with s3 compatible api)
	wg.Go(func() error { 
		return uploadS3(gctx, config.uploader, key, bytes.NewReader(data), metadata)
	})
	
	wg.Go(func() error {
		return postToSlack(gctx, config, key, bytes.NewReader(data), metadata)
	})
	err = wg.Wait()
	return msg, err
}

<<<<<<< Updated upstream
=======
func gemini(ctx context.Context, data []byte) (string, error) {

	client, err := genai.NewClient(ctx, option.WithAPIKey(geminiApiKey))
	if err != nil {
		return "", err
	}

	prompt := strings.Join(append(streets, append(modifiers, terms...)...), ", ")
	parts := []genai.Part{
		genai.Blob{MIMEType: "audio/mp3", Data: data},
		genai.Text("Transcribe the audio."),
		genai.Text("Ignore silences."),
		genai.Text("Here are some correction terms: " + terms),
	}

	model := client.GenerativeModel("gemini-1.5-pro")
	// Generate content using the prompt.
	resp, err := model.GenerateContent(ctx, parts...)
	if err != nil {
		log.Fatal(err)
	}

	// Handle the response of generated text

	b, _ := json.MarshalIndent(resp, "", " ")
	fmt.Println(string(b))

	var buf strings.Builder

	for _, c := range resp.Candidates {
		if c.Content != nil {
			for _, part := range c.Content.Parts {
				fmt.Fprintf(&buf, "%v", part)
				buf.WriteString("\n")
			}
		}
	}

	fmt.Println(buf.String())
	return buf.String(), nil
}

>>>>>>> Stashed changes
// whisper transcribes the audio with cloudflare Whisper
func whisper(ctx context.Context, data []byte) (string, error) {
	prompt := strings.Join(append(streets, append(modifiers, terms...)...), ", ")	
	
	enc := base64.StdEncoding.EncodeToString(data)
	payload, err := json.Marshal(CloudflareWhisperInput {
		Audio: enc,
		Prompt: prompt,
	})
	
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", cloudflareWhisperUrl, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer " + cloudflareApiToken)
		
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("Error calling cloudflare: %v\n", err)
		return "", err
	}
	defer resp.Body.Close()
	var output CloudflareWhisperOutput
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	err = json.Unmarshal(body, &output)
	if err != nil {
		return "", err
	}
	fmt.Println("Response from cloudflare: ", output)
	return output.Result.Text, nil	
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
	fmt.Println("done uploading")
	return err
}

func postToSlack(ctx context.Context, config *Config, key string, reader io.Reader, meta Metadata) error {
	if config.slackClient == nil {
		log.Println("Missing SLACK_API_SECRET or SLACK_WEBHOOK_URL. Slack notifications disabled.")
		return nil
	}
	if meta.AudioText == "" {
		meta.AudioText = "Could not transcribe audio"
	}

	blocks := strings.Split(meta.AudioText, ". ")

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
		block = strings.TrimSpace(block) + "."
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
	file, err := config.slackClient.UploadFile(slack.FileUploadParameters{
		Filename:       filename,
		Filetype:       "auto",
		Reader:         reader,
		InitialComment: sentances,
		Channels:       []string{string(channelID)},
	})
	if err != nil {
		log.Println("Error uploading file to slack: ", err)
	} else {
		log.Printf("Uploaded file: %s of type: %s to channel: %v", file.Name, file.Filetype, channelID)
	}

	return err
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
		matched := false
		for _, keyword := range notifs.Include {
			keyword = strings.ToLower(keyword)
			for _, word := range words {
				if keyword == word {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if matched {
			slackMeta.Mentions = append(slackMeta.Mentions, "<@"+string(userID)+">")
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

// Structures to parse json of the form:
//
//	{
//	  "freq": 772693750,
//	  "start_time": 1702513859,
//	  "stop_time": 1702513867,
//	  "emergency": 0,
//	  "priority": 4,
//	  "mode": 0,
//	  "duplex": 0,
//	  "encrypted": 0,
//	  "call_length": 6,
//	  "talkgroup": 3105,
//	  "talkgroup_tag": "Berkeley PD1",
//	  "talkgroup_description": "Police Dispatch",
//	  "talkgroup_group_tag": "Law Dispatch",
//	  "talkgroup_group": "Berkeley",
//	  "audio_type": "digital",
//	  "short_name": "Berkeley",
//	  "freqList": [
//	    {
//	      "freq": 772693750,
//	      "time": 1702513859,
//	      "pos": 0,
//	      "len": 1.08,
//	      "error_count": "100",
//	      "spike_count": "7"
//	    },
//	    {
//	      "freq": 772693750,
//	      "time": 1702513862,
//	      "pos": 1.08,
//	      "len": 3.78,
//	      "error_count": "21",
//	      "spike_count": "1"
//	    },
//	  ],
//	  "srcList": [
//	    {
//	      "src": 3113003,
//	      "time": 1702513859,
//	      "pos": 0,
//	      "emergency": 0,
//	      "signal_system": "",
//	      "tag": "Dispatch"
//	    },
//	    {
//	      "src": 3124119,
//	      "time": 1702513862,
//	      "pos": 1.08,
//	      "emergency": 0,
//	      "signal_system": "",
//	      "tag": ""
//	    },
//	  ]
//	}
type Frequency struct {
	Freq int64   `json:"freq,omitempty"`
	Time int64   `json:"time,omitempty"`
	Pos  float64 `json:"pos,omitempty"`
}
type Source struct {
	Src          int64   `json:"src,omitempty"`
	Time         int64   `json:"time,omitempty"`
	Pos          float64 `json:"pos,omitempty"`
	Emergency    int64   `json:"emergency,omitempty"`
	SignalSystem string  `json:"signal_system,omitempty"`
	Tag          string  `json:"tag,omitempty"`
}

type Metadata struct {
	Freq              int64       `json:"freq,omitempty"`
	StartTime         int64       `json:"start_time,omitempty"`
	StopTime          int64       `json:"stop_time,omitempty"`
	Emergency         int64       `json:"emergency,omitempty"`
	Priority          int64       `json:"priority,omitempty"`
	Mode              int64       `json:"mode,omitempty"`
	Duplex            int64       `json:"duplex,omitempty"`
	Encrypted         int64       `json:"encrypted,omitempty"`
	CallLength        int64       `json:"call_length,omitempty"`
	Talkgroup         int64       `json:"talkgroup,omitempty"`
	TalkgroupTag      string      `json:"talkgroup_tag,omitempty"`
	TalkGroupDesc     string      `json:"talkgroup_description,omitempty"`
	TalkGroupGroupTag string      `json:"talkgroup_group_tag,omitempty"`
	TalkGroupGroup    string      `json:"talkgroup_group,omitempty"`
	AudioType         string      `json:"audio_type,omitempty"`
	ShortName         string      `json:"short_name,omitempty"`
	AudioText         string      `json:"audio_text,omitempty"`
	URL               string      `json:"url,omitempty"`
	SrcList           []Source    `json:"srcList,omitempty"`
	FreqList          []Frequency `json:"freqList,omitempty"`
}

type Notifs struct {
	Include  []string
	Regex    *regexp.Regexp
	NotRegex *regexp.Regexp
	Channels []SlackChannelID //the channels to listen on
}

type SlackMeta struct {
	Mentions []string `json:"mentions,omitempty"`
	Address  Address  `json:"address,omitempty"`
}

// Address struct encapsulates address info to extract from transcription text
// Example transcriptions: "2605 Durant", "Can you start for Russell and California please?"
type Address struct {
	City           string   `json:"city,omitempty"`
	PrimaryAddress string   `json:"primary,omitempty"`
	Streets        []string `json:"streets,omitempty"`
}

func (addr Address) AppendStreet(street string) Address {
	for _, st := range addr.Streets {
		if st == street {
			return addr
		}
	}
	addr.Streets = append(addr.Streets, street)
	return addr
}

func (addr Address) String() string {
	switch {
	case addr.PrimaryAddress != "":
		return addr.PrimaryAddress
	case len(addr.Streets) > 1:
		return addr.Streets[0] + " and " + addr.Streets[1]
	default:
		return ""
	}
}


type TranscriptionInfo struct {
	Language string  `json:"language,omitempty"`
	Duration string  `json:"duration,omitempty"`
	Text     string  `json:"text,omitempty"`
	WordCount int    `json:"word_count,omitempty"`
}

type CloudflareWhisperInput struct {
	Audio string 	`json:"audio,omitempty"`
	Prompt string 	`json:"initial_prompt,omitempty"`
	Prefix string   `json:"prefix,omitempty"`
}

type CloudflareWhisperOutput struct {
	Result TranscriptionInfo `json:"result,omitempty"`
	Success bool             `json:"success,omitempty"`
	Errors  []string         `json:"errors,omitempty"`
	Messages []string        `json:"messages,omitempty"`
}
