# Konnectors workflow

## Types

### Accounts

`io.cozy.accounts` contains authentification information for an account

```json
{
    "name" : "Gmail Perso",
    "accountType": "google",
    "status": "connected",
    "auth": {
      "user": "my-personal-account@gmail.com",
      "password": "my-secret"
    }
}
```

accounts are manipulated through the `/data/` API.

#### Accounts fields  
- **name** User defined name for the account ("Perso", "Pro")
- **accountType**  
- **status** one of "NoAttempt" "Connected" or "Errored"
- **error** the (optional) error for last connection to this account
- **auth** An object defining auth method for this account. For now only        {login, password} is suppoted.


OAuth accounts will be explored later.
The auth fields will be encrypted on disk.

Account permission should appears different in permission modal.


### Konnectors

`io.cozy.konnectors` are installed similarly to `io.cozy.apps` (see [doc](./konnectors.md))


### Permissions

Like client-side applications, each konnectors has an associated `io.cozy.permissions` with `type=app` doc.

### Triggers

`io.cozy.triggers` are used to define when konnectors are launched
See https://cozy.github.io/cozy-stack/jobs.html#post-jobstriggers


------------------------------------

## Routes

**konnectors**
- [ ] `GET    /konnectors/marketplace` Lists available konnectors
- [x] `POST   /konnectors?Source=xxxx` Installs a konnector            
- [ ] `GET    /konnectors`             Lists installed konnectors    

**triggers**
- [ ] `GET    /jobs/triggers?worker=konnector` Lists active konnectors       
- [x] `POST   /jobs/triggers`                  Enables a konnector recurrence            
- [x] `DELETE /jobs/triggers/:triggerid` Disables a konnector recurrence            

**jobs**
- [x] `POST   /jobs/queue/konnector` Starts a konnector now        
- [x] `GET    /jobs/queue/konnector` Lists pending konnectors       


--------------------------------------


## Complete flow example

As a user, From the expenses management app, I have clean flow to configure a connector to retrieve my travel expenses

1 - User is in **my-expenses** and clicks on [configure travels]
2 - **my-expenses** triggers an intent

```javascript
cozy.intents.start('CREATE', 'io.cozy.konnectors', {    
    category: 'transport'
})
```

3 - SettingsApp catch the intent, fetch all available konnectors and let the user choose

```http
GET /konnectors/marketplace
```


4 - SettingsApp fetch selected konnector (trainlines) manifest

```http
GET /konnectors/manifests?Source=git://github.com/konnectors/trainlines.git
```
```json
{
  "name": "Trainline",
  "type": "konnector",
  "slug": "konnector-trainline",
  "description": "Konnector for trainline . com",
  "source": "https://github.com/konnectors/trainlines.git@build",
  "developer": {
    "name": "XXX",
    "url": "https://www.xxx.fr"
  },
  "version": "3.0.0",
  "licence": "AGPL-3.0",
  "fields": {
    "saveFolder": {
      "doctype": "io.cozy.files",
      "type": "folder",
      "verbs":["ALL"]
    },
    "account": {
      "doctype": "io.cozy.accounts",
      "accountType": "trainlines",
      "accountFormat": "login,password",
    }
  },
  "category": "transport",
  "frequency": "weekly",
  "permissions": {
    "events": {
      "description": "Connect train bill with  event in your calendar",
      "type": "io.cozy.events",
      "verbs": ["PATCH"]
    }
  }
}
```

5 - SettingsApp asks the user for account config and create the io.cozy.accounts

```http
POST /data/io.cozy.accounts
```
```json
{
  "accountType": "google",
  "status": "PENDING",
  "auth": {
    "login": "xxxx",
    "password": "yyyyy",
  }
}
```
```http
HTTP/1.1 200 OK
```
```json
{
  "_id":"123-account-id-123",
  "_rev":"1-asasasasa",
  "accountType": "google",
  "status": "PENDING",
  "auth": {
    "login": "xxxx",
    "password": "yyyyy",
  }
}
```

6 - SettingsApp asks the user for the additional "saveFolder" config fields. It could for example use a PICK intents for files.

7 - SettingsApp does install the konnector

```http
POST /konnectors/konnector-trainlines?Source=git://github.com/konnectors/trainlines.git
```
```http
HTTP/1.1 200 OK
```
```json
{
  "data": {
    "id":"io.cozy.konnectors/trainlines",
    "type":"io.cozy.konnectors",
    "attributes": {
      "name": "trainline",
      "state": "installing",
      "slug": "trainline",
      ...
    },
    "links": {
      "self":"/konnectors/trainline",
      "permissions":"/permissions/456-permission-doc-id-456"
    }
  }
}
```

8 - SettingsApp change the konnector permissions doc to include save folder

```http
PATCH /permissions/456-permission-doc-id-456
```
```json
{
  "data": {
    "id": "456-permission-doc-id-456",
    "type": "io.cozy.permissions",
    "attributes": {
      "type": "app",
      "source_id": "io.cozy.konnectors/trainlines",
      "permissions": {
        "saveFolder": {
          "type": "io.cozy.files",
          "verbs": ["ALL"],
          "values": ["123-selected-folder-id-123"]
        }
      }
    }
  }
}
```
```http
HTTP/1.1 200 OK
```
```json
{
  "data": {
    "id": "456-permission-doc-id-456",
    "type": "io.cozy.permissions",
    "attributes": {
      "type": "app",
      "source_id": "io.cozy.konnectors/trainlines",
      "permissions": {
        "events": {
          "description": "Connect train bill with  event in your calendar",
          "type": "io.cozy.events",
          "verbs": ["PATCH"]
        },
        "saveFolder": {
          "type": "io.cozy.files",
          "verbs": ["ALL"],
          "values": ["123-selected-folder-id-123"]
        }
      }
    }
  }
}
```       

9 - SettingsApp add a Reference from konnector to folder to prevent folder destruction

```http
POST /files/123-selected-folder-id-123/relationships/referenced_by
```
```json
{
  "data": [
    {
      "type": "io.cozy.konnectors",
      "id": "io.cozy.konnectors/trainlines"
    }
  ]
}
```

10 - SettingsApp runs the konnector the first time

```http
POST /jobs/queue/konnector
```
```json
{
  "data": {
    "attributes": {
       "options": {
       "priority": 10,
       "timeout": 180,
       "max_exec_count": 1
      }
    },
    "arguments": {
      "konnector": "io.cozy.konnectors/trainlines",
      "account": "123-account-id-123",
      "folderToSave": "123-selected-folder-id-123"
    }
  }
}
```
```http
HTTP/1.1 200 OK
```
```json
{
  "data": {
    "id":"789-job-id-789",
    "type":"io.cozy.jobs",
    "attributes": {
      "worker": "konnector",
      "options": {
        "priority": 3,
        "timeout": 60,
        "max_exec_count": 3
      },
      "arguments": {
        "konnector": "io.cozy.konnectors/trainlines",
        "account": "123-account-id-123",
        "folderToSave": "123-selected-folder-id-123"
      },
      "state": "running",
      "try_count": 1,
      "queued_at": "2016-09-19T12:35:08Z",
      "started_at": "2016-09-19T12:35:08Z",
      "errors": [],
      "output": {}
    }
  }
}
```

11 - SettingsApp follow konnector status to check if all is properly configured.
It uses the [realtime](./realtime.md) to subscribes to changes on `789-job-id-789` and is therefore informed of it's status / errors / ect ...


**TODO** Look at current konnectors sources to defines a protocol between konnectors and SettingsApp to display the nice progress modal.
  - [ ] 250 events imported
  - [ ] 150 / 3500 contacts importing
  - [ ] ...

**TODO** there should be some persistence for jobs error / status


12 - SettingsApp creates a trigger to setup the konnector recurence

```http
POST /jobs/io.cozy.triggers
```
```json
{
  "data": {
    "attributes": {
      "type": "@cron",
      "arguments": "0 0 0 0 1 1 ",
      "worker": "konnector",
      "worker_arguments": {
        "konnector": "trainlines",
        "account": "5165621628784562148955",
        "folderToSave": "877854878455",
      }
    }
  }
}
```

If the user wants to use several account, Settings can setup several triggers for the same konnector & various accounts.


### Relations and DELETE CASCADE

- When a konnector is deleted, its triggers should be deleted
- When an account is deleted, its triggers should be deleted


**TODO** Should stack validate konnector value on trigger creation / to be extended to all workers pre-validating worker arguments ?


## Konnector Worker specs

- Start the konnector through Rkt, passing token as ENV variable
- Pass the rest of worker_arguments to the konnector (ENV / stdin ?)
- Use realtime to send konnector status back (stdout / http) ?
