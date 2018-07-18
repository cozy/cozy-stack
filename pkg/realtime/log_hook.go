package realtime

import (
	"time"

	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/sirupsen/logrus"
)

type log struct {
	LogID   string                 `json:"_id"`
	Time    time.Time              `json:"time"`
	Message string                 `json:"message"`
	Level   string                 `json:"level"`
	Data    map[string]interface{} `json:"data"`

	docType string
}

func (l *log) DocType() string   { return l.docType }
func (l *log) ID() string        { return l.LogID }
func (l *log) Rev() string       { return "" }
func (l *log) SetID(id string)   { l.LogID = id }
func (l *log) SetRev(rev string) {}

type logHook struct {
	Hub
	db      prefixer.Prefixer
	docType string
	docID   string
}

// LogHook creates a hook that transmits logs through redis pubsub
// messaging.
func LogHook(db prefixer.Prefixer, hub Hub, parentDocType, documentID string) logrus.Hook {
	return &logHook{
		Hub:     hub,
		db:      db,
		docType: parentDocType + ".logs",
		docID:   documentID,
	}
}

func (r *logHook) Levels() []logrus.Level {
	return []logrus.Level{
		logrus.DebugLevel,
		logrus.InfoLevel,
		logrus.WarnLevel,
		logrus.ErrorLevel,
		logrus.FatalLevel,
		logrus.PanicLevel,
	}
}

func (r *logHook) Fire(entry *logrus.Entry) error {
	doc := &log{
		LogID:   r.docID,
		Time:    entry.Time,
		Message: entry.Message,
		Level:   entry.Level.String(),
		Data:    entry.Data,
		docType: r.docType,
	}
	r.Hub.Publish(r.db, EventCreate, doc, nil)
	return nil
}
