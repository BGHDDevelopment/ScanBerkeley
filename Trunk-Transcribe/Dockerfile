FROM golang:1.23.0

RUN apt-get update -qq && apt-get install ffmpeg -y

# Set destination for COPY
WORKDIR /app

# Download Go modules
COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./
RUN mkdir -p templates
COPY templates/* templates
COPY deep-filter ./

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -o /transcribe

EXPOSE 8080

# Run
CMD ["/transcribe"]
