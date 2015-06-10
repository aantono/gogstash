package inputdocker

import (
	"bytes"
	"io"
	"log"
	"os"
	"regexp"
	"time"

	"github.com/tsaikd/gogstash/config"
)

func NewContainerLogStream(eventChan chan config.LogEvent, id string, eventExtra map[string]interface{}, since *time.Time, logger *log.Logger) ContainerLogStream {
	if logger == nil {
		logger = log.New(os.Stdout, "", log.LstdFlags)
	}
	return ContainerLogStream{
		ID:         id,
		eventChan:  eventChan,
		eventExtra: eventExtra,
		logger:     logger,
		buffer:     bytes.NewBuffer(nil),

		since: since,
	}
}

type ContainerLogStream struct {
	io.Writer
	ID         string
	eventChan  chan config.LogEvent
	eventExtra map[string]interface{}
	logger     *log.Logger
	buffer     *bytes.Buffer
	since      *time.Time
}

func (t *ContainerLogStream) Write(p []byte) (n int, err error) {
	n, err = t.buffer.Write(p)
	if err != nil {
		t.logger.Fatal(err)
		return
	}

	idx := bytes.IndexByte(t.buffer.Bytes(), '\n')
	for idx > 0 {
		data := t.buffer.Next(idx)
		err = t.sendEvent(data)
		t.buffer.Next(1)
		if err != nil {
			t.logger.Fatal(err)
			return
		}
		idx = bytes.IndexByte(t.buffer.Bytes(), '\n')
	}
	return
}

var (
	reTime = regexp.MustCompile(`[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\.[0-9]+Z[0-9+-]*`)
)

func (t *ContainerLogStream) sendEvent(data []byte) (err error) {
	var (
		eventTime time.Time
	)

	event := config.LogEvent{
		Timestamp: time.Now(),
		Message:   string(data),
		Extra:     t.eventExtra,
	}

	event.Extra["containerid"] = t.ID

	loc := reTime.FindIndex(data)
	if loc[0] < 10 {
		timestr := string(data[loc[0]:loc[1]])
		eventTime, err = time.Parse(time.RFC3339Nano, timestr)
		if err == nil {
			if eventTime.Before(*t.since) {
				return
			}
			event.Timestamp = eventTime
			data = data[loc[1]+1:]
		} else {
			t.logger.Println(err)
		}
	} else {
		t.logger.Fatal("invalid event format", string(data))
	}

	event.Message = string(bytes.TrimSpace(data))

	if t.since.Before(event.Timestamp) {
		*t.since = event.Timestamp
	} else {
		return
	}

	if err != nil {
		event.AddTag("inputdocker_failed")
		err = nil
	}

	t.eventChan <- event

	return
}
