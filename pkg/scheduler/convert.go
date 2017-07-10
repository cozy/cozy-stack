package scheduler

import (
	"time"

	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/logger"
)

// ImportFromMemStorage fetch triggers from the old storage and add them
// into the new scheduler
func (sched *RedisScheduler) ImportFromMemStorage() error {

	oldStorage := &globalDBStorage{}

	ts, err := oldStorage.GetAll()
	if err != nil {
		return nil
	}

	for _, t := range ts {
		domain := t.Domain

		_, err := sched.Get(domain, t.ID())
		switch err {
		case nil:
			// trigger already copied
			errd := couchdb.DeleteDoc(couchdb.GlobalTriggersDB, t)
			if errd != nil {
				logger.WithNamespace("event-trigger").Errorln("error deleting old trigger", err)
			}
			continue

		case ErrNotFoundTrigger:
			// trigger does not exists in new storage, copy it
			trigger, errn := NewTrigger(t)
			if errn != nil {
				logger.WithDomain(t.Domain).Errorln("old trigger is invalid", t)
				continue
			}

			db := couchdb.SimpleDatabasePrefix(t.Domain)
			revback := t.Rev()
			t.TRev = ""
			if err = couchdb.CreateNamedDocWithDB(db, t); err != nil {
				return err
			}
			t.SetRev(revback)

			if err = sched.addToRedis(trigger, time.Now()); err != nil {
				return err
			}

			if err = oldStorage.Delete(trigger); err != nil {
				return err
			}

		default:
			return err
		}
	}

	return nil
}

// ImportFromDB fetch the triggers from couchdb an reimport them into redis
func (sched *RedisScheduler) ImportFromDB(domain string) error {

	db := couchdb.SimpleDatabasePrefix(domain)
	var docs []*TriggerInfos
	req := &couchdb.AllDocsRequest{Limit: 100}
	if err := couchdb.GetAllDocs(db, consts.Triggers, req, &docs); err != nil {
		return err
	}

	for _, d := range docs {
		t, err := NewAtTrigger(d)
		if err != nil {
			logger.WithDomain(domain).Errorln("Bad trigger in couchdb: ", t, err)
		} else {
			sched.addToRedis(t, time.Now())
		}
	}

	return nil
}
