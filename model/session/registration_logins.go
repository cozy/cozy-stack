package session

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/cozy/cozy-stack/model/instance/lifecycle"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/pkg/consts"
	"github.com/cozy/cozy-stack/pkg/couchdb"
	"github.com/cozy/cozy-stack/pkg/logger"
	"github.com/cozy/cozy-stack/pkg/prefixer"
	"github.com/cozy/cozy-stack/pkg/utils"
)

var log = logger.WithNamespace("sessions")

const redisRegistationKey = "registration-logins"

type registrationEntry struct {
	Domain       string
	ClientID     string
	LoginEntryID string
	Expire       time.Time
}

func (r *registrationEntry) Key() string {
	return r.Domain + "|" + r.ClientID
}

var (
	registrationExpirationDuration = 5 * time.Minute

	registrationsMap     map[string]registrationEntry
	registrationsMapLock sync.Mutex
)

// SweepLoginRegistrations starts the login registration process.
//
// This process involving a queue of registration login entries is necessary to
// distinguish "normal" logins from logins to give right to an OAuth
// application.
//
// Since we cannot really distinguish between them other than trusting the
// user, we send a notification to the user by following this process:
//   - if we identify a login for a device registration — by looking at the
//     redirection address — we push an entry onto the queue
//   - if we do not receive the activation of the device by the user in 5
//     minutes, we send a notification for a "normal" login
//   - otherwise we send a notification for the activation of a new device.
func SweepLoginRegistrations() utils.Shutdowner {
	closed := make(chan struct{})
	go func() {
		waitDuration := registrationExpirationDuration / 2
		for {
			select {
			case <-time.After(waitDuration):
				var err error
				waitDuration, err = sweepRegistrations()
				if err != nil {
					log.Errorf("Could not sweep registration queue: %s", err)
				}
				if waitDuration <= 0 {
					waitDuration = registrationExpirationDuration
				}
			case <-closed:
				return
			}
		}
	}()
	return &sweeper{closed}
}

type sweeper struct {
	closed chan struct{}
}

func (s *sweeper) Shutdown(ctx context.Context) error {
	select {
	case s.closed <- struct{}{}:
	case <-ctx.Done():
	}
	return nil
}

// PushLoginRegistration pushes a new login into the registration queue.
func PushLoginRegistration(db prefixer.Prefixer, login *LoginEntry, clientID string) error {
	entry := registrationEntry{
		Domain:       db.DomainName(),
		ClientID:     clientID,
		LoginEntryID: login.ID(),
		Expire:       time.Now(),
	}

	if cli := config.GetConfig().SessionStorage.Client(); cli != nil {
		b, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		return cli.HSet(redisRegistationKey, entry.Key(), b).Err()
	}

	registrationsMapLock.Lock()
	if registrationsMap == nil {
		registrationsMap = make(map[string]registrationEntry)
	}
	registrationsMap[entry.Key()] = entry
	registrationsMapLock.Unlock()
	return nil
}

// RemoveLoginRegistration removes a login from the registration map.
func RemoveLoginRegistration(domain, clientID string) error {
	var entryPtr *registrationEntry
	key := domain + "|" + clientID
	if cli := config.GetConfig().SessionStorage.Client(); cli != nil {
		b, err := cli.HGet(redisRegistationKey, key).Result()
		if err != nil {
			return err
		}
		var entry registrationEntry
		if err = json.Unmarshal([]byte(b), &entry); err != nil {
			return err
		}
		if err = cli.HDel(redisRegistationKey, key).Err(); err != nil {
			return err
		}
		entryPtr = &entry
	} else {
		registrationsMapLock.Lock()
		entry, ok := registrationsMap[key]
		if ok {
			delete(registrationsMap, key)
			entryPtr = &entry
		}
		registrationsMapLock.Unlock()
	}
	if entryPtr != nil {
		_ = sendRegistrationNotification(entryPtr, true)
	}
	return nil
}

func sweepRegistrations() (waitDuration time.Duration, err error) {
	var expiredLogins []registrationEntry

	now := time.Now()
	if cli := config.GetConfig().SessionStorage.Client(); cli != nil {
		var vals map[string]string
		vals, err = cli.HGetAll(redisRegistationKey).Result()
		if err != nil {
			return
		}

		var deletedKeys []string
		for key, data := range vals {
			var entry registrationEntry
			if err = json.Unmarshal([]byte(data), &entry); err != nil {
				deletedKeys = append(deletedKeys, key)
				continue
			}
			diff := entry.Expire.Sub(now)
			if diff < -24*time.Hour {
				// skip too old entries
				deletedKeys = append(deletedKeys, entry.Key())
			} else if diff <= 10*time.Second {
				expiredLogins = append(expiredLogins, entry)
				deletedKeys = append(deletedKeys, entry.Key())
			} else if waitDuration == 0 || waitDuration > diff {
				waitDuration = diff
			}
		}

		if len(deletedKeys) > 0 {
			err = cli.HDel(redisRegistationKey, deletedKeys...).Err()
		}
	} else {
		registrationsMapLock.Lock()

		var deletedKeys []string
		for _, entry := range registrationsMap {
			diff := entry.Expire.Sub(now)
			if diff < -24*time.Hour {
				// skip too old entries
				deletedKeys = append(deletedKeys, entry.Key())
			} else if diff <= 10*time.Second {
				expiredLogins = append(expiredLogins, entry)
				deletedKeys = append(deletedKeys, entry.Key())
			} else if waitDuration == 0 || waitDuration > diff {
				waitDuration = diff
			}
		}

		for _, key := range deletedKeys {
			delete(registrationsMap, key)
		}

		registrationsMapLock.Unlock()
	}

	if len(expiredLogins) > 0 {
		sendExpiredRegistrationNotifications(expiredLogins)
	}

	return
}

func sendRegistrationNotification(entry *registrationEntry, registrationNotification bool) error {
	var login LoginEntry
	i, err := lifecycle.GetInstance(entry.Domain)
	if err != nil {
		return err
	}
	err = couchdb.GetDoc(i, consts.SessionsLogins, entry.LoginEntryID, &login)
	if err != nil {
		return err
	}
	var clientID string
	if registrationNotification {
		clientID = entry.ClientID
	}
	return sendLoginNotification(i, &login, clientID)
}

func sendExpiredRegistrationNotifications(entries []registrationEntry) {
	for _, entry := range entries {
		_ = sendRegistrationNotification(&entry, false)
	}
}
