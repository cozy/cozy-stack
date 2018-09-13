[Table of contents](README.md#table-of-contents)

# Jobs

Jobs are designed to represent asynchronous tasks that your cozy can execute.
These tasks can be scheduled in advance, recurring or sudden and provide various
services.

At the time, we do not provide "generic" and programmable tasks. This list of
available workers is part of the defined API.

The job queue can be either local to the cozy-stack when used as a monolithic
instance (self-hosted for instance) or distributed via a redis server for
distributed infrastructures.

This doc introduces two cozy types:

* `io.cozy.jobs` for jobs
* `io.cozy.triggers` for triggers

## Triggers

Jobs can be launched by five different types of triggers:

* `@at` to schedule a one-time job executed after at a specific time in the
  future
* `@in` to schedule a one-time job executed after a specific amount of time
* `@every` to schedule periodic jobs executed at a given fix interval
* `@cron` to schedule recurring jobs scheduled at specific times
* `@event` to launch a job after a change in the cozy

These five triggers have specific syntaxes to describe when jobs should be
scheduled. See below for more informations.

Jobs can also be queued up programatically, without the help of a specific
trigger directly via the `/jobs/queue` API. In this case the `trigger` name is
empty.

### `@at` syntax

The `@at` trigger takes a ISO-8601 formatted string indicating a UTC time in the
future. The date is of this form: `YYYY-MM-DDTHH:mm:ss.sssZ`

Examples

```
@at 2018-12-12T15:36:25.507Z
```

### `@in` syntax

The `@in` trigger takes the same duration syntax as `@every`

Examples

```
@in 10m
@in 1h30m
```

### `@every` syntax

The `@every` trigger uses the same syntax as golang's `time.ParseDuration` (but
only support time units above seconds):

A duration string is a possibly signed sequence of decimal numbers, each with
optional fraction and a unit suffix, such as "300ms", "1.5h" or "2h45m". Valid
time units are "s", "m", "h".

Examples

```
@every 1.5h   # schedules every 1 and a half hours
@every 30m10s # schedules every 30 minutes and 10 seconds
```

### `@cron` syntax

In order to schedule recurring jobs, the `@cron` trigger has the syntax using
six fields:

| Field name   | Mandatory? | Allowed values  | Allowed special characters |
| ------------ | ---------- | --------------- | -------------------------- |
| Seconds      | Yes        | 0-59            | \* / , -                   |
| Minutes      | Yes        | 0-59            | \* / , -                   |
| Hours        | Yes        | 0-23            | \* / , -                   |
| Day of month | Yes        | 1-31            | \* / , - ?                 |
| Month        | Yes        | 1-12 or JAN-DEC | \* / , -                   |
| Day of week  | Yes        | 0-6 or SUN-SAT  | \* / , - ?                 |

Asterisk ( `*` )

The asterisk indicates that the cron expression will match for all values of the
field; e.g., using an asterisk in the 5th field (month) would indicate every
month.

Slash ( `/` )

Slashes are used to describe increments of ranges. For example 3-59/15 in the
1st field (minutes) would indicate the 3rd minute of the hour and every 15
minutes thereafter. The form `"*\/..."` is equivalent to the form
`"first-last/..."`, that is, an increment over the largest possible range of the
field. The form `"N-/..."` is accepted as meaning `"N-MAX/..."`, that is,
starting at N, use the increment until the end of that specific range. It does
not wrap around.

Comma ( `,` )

Commas are used to separate items of a list. For example, using `"MON,WED,FRI"`
in the 5th field (day of week) would mean Mondays, Wednesdays and Fridays.

Hyphen ( `-` )

Hyphens are used to define ranges. For example, 9-17 would indicate every hour
between 9am and 5pm inclusive.

Question mark ( `?` )

Question mark may be used instead of `*` for leaving either day-of-month or
day-of-week blank.

To schedule jobs given an interval:

Examples:

```
@cron 0 0 0 1 1 *  # Run once a year, midnight, Jan. 1st
@cron 0 0 0 1 1 *  # Run once a year, midnight, Jan. 1st
@cron 0 0 0 1 * *  # Run once a month, midnight, first of month
@cron 0 0 0 * * 0  # Run once a week, midnight on Sunday
@cron 0 0 0 * * *  # Run once a day, midnight
@cron 0 0 * * * *  # Run once an hour, beginning of hour
```

### `@event` syntax

The `@event` syntax allows to trigger a job when something occurs in the stack.
It follows the same syntax than permissions scope string:

`type[:verb][:values][:selector]`

Unlike for permissions string, the verb should be one of `CREATED`, `DELETED`,
`UPDATED`.

The job worker will receive a compound message including original trigger_infos
messages and the event which has triggered it.

Examples

```
@event io.cozy.files // anything happens on files
@event io.cozy.files:CREATED // a file was created
@event io.cozy.files:DELETED:image/jpg:mime // an image was deleted
@event io.cozy.bank.operations:CREATED io.cozy.bank.bills:CREATED // a bank operation or a bill
```

## Error Handling

Jobs can fail to execute their task. We have two ways to parameterize such
cases.

### Retry

A retry count can be optionally specified to ask the worker to re-execute the
task if it has failed.

Each retry is executed after a configurable delay. The try count is part of the
attributes of the job. Also, each occurring error is kept in the `errors` field
containing all the errors that may have happened.

### Timeout

A worker may never end. To prevent this, a configurable timeout value is
specified with the job.

If a job does not end after the specified amount of time, it will be aborted. A
timeout is just like another error from the worker and can provoke a retry if
specified.

### Defaults

By default, jobs are parameterized with a maximum of 3 tries with 1 minute
timeout.

These defaults may vary given the workload of the workers.

## Jobs API

Example and description of the attributes of a `io.cozy.jobs`:

```js
{
  "domain": "me.cozy.tools",
  "worker": "sendmail",    // worker type name
  "options": {
    "priority": 3,         // priority from 1 to 100, higher number is higher priority
    "timeout": 60,         // timeout value in seconds
    "max_exec_count": 3,   // maximum number of time the job should be executed (including retries)
  },
  "state": "running",      // queued, running, errored
  "queued_at": "2016-09-19T12:35:08Z",  // time of the queuing
  "started_at": "2016-09-19T12:35:08Z", // time of first execution
  "error": ""             // error message if any
}
```

Example and description of a job creation options — as you can see, the options
are replicated in the `io.cozy.jobs` attributes:

```js
{
  "priority": 3,         // priority from 1 to 100
  "timeout": 60,         // timeout value in seconds
  "max_exec_count": 3,   // maximum number of retry
}
```

### GET /jobs/:job-id

Get a job informations given its ID.

#### Request

```http
GET /jobs/123123 HTTP/1.1
Accept: application/vnd.api+json
```

#### Response

```json
{
  "data": {
    "type": "io.cozy.jobs",
    "id": "123123",
    "attributes": {
      "domain": "me.cozy.tools",
      "worker": "sendmail",
      "options": {
        "priority": 3,
        "timeout": 60,
        "max_exec_count": 3
      },
      "state": "running",
      "queued_at": "2016-09-19T12:35:08Z",
      "started_at": "2016-09-19T12:35:08Z",
      "error": ""
    },
    "links": {
      "self": "/jobs/123123"
    }
  }
}
```

### POST /jobs/queue/:worker-type

Enqueue programmatically a new job.

This route requires a specific permission on the worker-type. A global
permission on the global `io.cozy.jobs` doctype is not allowed.

#### Request

```http
POST /jobs/queue/sendmail HTTP/1.1
Accept: application/vnd.api+json
```

```json
{
  "data": {
    "attributes": {
      "options": {
        "priority": 3,
        "timeout": 60,
        "max_exec_count": 3
      },
      "arguments": {} // any json value used as arguments for the job
    }
  }
}
```

#### Response

```json
{
  "data": {
    "type": "io.cozy.jobs",
    "id": "123123",
    "attributes": {
      "domain": "me.cozy.tools",
      "worker": "sendmail",
      "options": {
        "priority": 3,
        "timeout": 60,
        "max_exec_count": 3
      },
      "state": "running",
      "queued_at": "2016-09-19T12:35:08Z",
      "started_at": "2016-09-19T12:35:08Z",
      "error": ""
    },
    "links": {
      "self": "/jobs/123123"
    }
  }
}
```

#### Permissions

To use this endpoint, an application needs a permission on the type
`io.cozy.jobs` for the verb `POST`. The is required to restrict its permission
to specific worker(s), like this (a global permission on the doctype is not
allowed):

```json
{
  "permissions": {
    "mail-from-the-user": {
      "description": "Required to send mails from the user to his/her friends",
      "type": "io.cozy.jobs",
      "verbs": ["POST"],
      "selector": "worker",
      "values": ["sendmail"]
    }
  }
}
```

### GET /jobs/queue/:worker-type

List the jobs in the queue.

#### Request

```http
GET /jobs/queue/sendmail HTTP/1.1
Accept: application/vnd.api+json
```

#### Response

```json
{
  "data": [
    {
      "attributes": {
        "domain": "cozy.tools:8080",
        "options": null,
        "queued_at": "2017-09-29T15:32:31.953878568+02:00",
        "started_at": "0001-01-01T00:00:00Z",
        "state": "queued",
        "worker": "log"
      },
      "id": "77689bca9634b4fb08d6ca3d1643de5f",
      "links": {
        "self": "/jobs/log/77689bca9634b4fb08d6ca3d1643de5f"
      },
      "meta": {
        "rev": "1-f823bcd2759103a5ad1a98f4bf083b36"
      },
      "type": "io.cozy.jobs"
    }
  ],
  "meta": {
    "count": 0
  }
}
```

#### Permissions

To use this endpoint, an application needs a permission on the type
`io.cozy.jobs` for the verb `GET`. In most cases, the application will restrict
its permission to only one worker, like this:

```json
{
  "permissions": {
    "mail-from-the-user": {
      "description":
        "Required to know the number of jobs in the sendmail queues",
      "type": "io.cozy.jobs",
      "verbs": ["GET"],
      "selector": "worker",
      "values": ["sendmail"]
    }
  }
}
```

### POST /jobs/triggers

Add a trigger of the worker. See [triggers' descriptions](#triggers) to see the
types of trigger and their arguments syntax.

This route requires a specific permission on the worker type. A global
permission on the global `io.cozy.triggers` doctype is not allowed.

The `debounce` parameter can be used to limit the number of jobs created in a
burst. It delays the creation of the job on the first input by the given time
argument, and if the trigger has its condition matched again during this period,
it won't create another job. It can be useful to combine it with the changes
feed of couchdb with a last sequence number persisted by the worker, as it
allows to have a nice diff between two executions of the worker.

#### Request

```http
POST /jobs/triggers HTTP/1.1
Accept: application/vnd.api+json
```

```json
{
  "data": {
    "attributes": {
      "type": "@event",
      "arguments": "io.cozy.invitations",
      "debounce": "10m",
      "worker": "sendmail",
      "worker_arguments": {},
      "options": {
        "priority": 3,
        "timeout": 60,
        "max_exec_count": 3
      }
    }
  }
}
```

#### Response

```json
{
  "data": {
    "type": "io.cozy.triggers",
    "id": "123123",
    "attributes": {
      "type": "@every",
      "arguments": "30m10s",
      "debounce": "10m",
      "worker": "sendmail",
      "options": {
        "priority": 3,
        "timeout": 60,
        "max_exec_count": 3
      }
    },
    "links": {
      "self": "/jobs/triggers/123123"
    }
  }
}
```

#### Permissions

To use this endpoint, an application needs a permission on the type
`io.cozy.triggers` for the verb `POST`. In most cases, the application will
restrict its permission to only one worker, like this:

```json
{
  "permissions": {
    "mail-from-the-user": {
      "description":
        "Required to send regularly mails from the user to his/her friends",
      "type": "io.cozy.triggers",
      "verbs": ["POST"],
      "selector": "worker",
      "values": ["sendmail"]
    }
  }
}
```

### GET /jobs/triggers/:trigger-id

Get a trigger informations given its ID.

#### Request

```http
GET /jobs/triggers/123123 HTTP/1.1
Accept: application/vnd.api+json
```

#### Response

```json
{
  "data": {
    "type": "io.cozy.triggers",
    "id": "123123",
    "attributes": {
      "type": "@every",
      "arguments": "30m10s",
      "worker": "sendmail",
      "options": {
        "priority": 3,
        "timeout": 60,
        "max_exec_count": 3
      },
      "current_state": {
        "status": "done",
        "last_success": "2017-11-20T13:31:09.01641731",
        "last_successful_job_id": "abcde",
        "last_execution": "2017-11-20T13:31:09.01641731",
        "last_executed_job_id": "abcde",
        "last_failure": "2017-11-20T13:31:09.01641731",
        "last_failed_job_id": "abcde",
        "last_error": "error value",
        "last_manual_execution": "2017-11-20T13:31:09.01641731",
        "last_manual_job_id": "abcde"
      }
    },
    "links": {
      "self": "/jobs/triggers/123123"
    }
  }
}
```

#### Permissions

To use this endpoint, an application needs a permission on the type
`io.cozy.triggers` for the verb `GET`.

### GET /jobs/triggers/:trigger-id/state

Get the trigger current state, to give a big picture of the health of the
trigger.

* last executed job status (`done`, `errored`, `queued` or `running`)
* last executed job that resulted in a successful executoin
* last executed job that resulted in an error
* last executed job from a manual execution (not executed by the trigger
  directly)

#### Request

```
GET /jobs/triggers/123123/state HTTP/1.1
Accept: application/vnd.api+json
```

#### Response

```json
{
  "data": {
    "type": "io.cozy.jobs.state",
    "id": "123123",
    "attributes": {
      "status": "done",
      "last_success": "2017-11-20T13:31:09.01641731",
      "last_successful_job_id": "abcde",
      "last_execution": "2017-11-20T13:31:09.01641731",
      "last_executed_job_id": "abcde",
      "last_failure": "2017-11-20T13:31:09.01641731",
      "last_failed_job_id": "abcde",
      "last_error": "error value",
      "last_manual_execution": "2017-11-20T13:31:09.01641731",
      "last_manual_job_id": "abcde"
    }
  }
}
```

#### Permissions

To use this endpoint, an application needs a permission on the type
`io.cozy.triggers` for the verb `GET`.

Also, for konnector trigger, any application sharing the same doctype
permissions as the konnector launched by the trigger will be granted the
permissions to fetch its state, even without permissions on the
`io.cozy.triggers` doctype.

### GET /jobs/triggers/:trigger-id/jobs

Get the jobs launched by the trigger with the specified ID.

Query parameters:

* `Limit`: to specify the number of jobs to get out

#### Request

```http
GET /jobs/triggers/123123/jobs?Limit=1 HTTP/1.1
Accept: application/vnd.api+json
```

#### Response

```json
{
  "data": [
    {
      "type": "io.cozy.jobs",
      "id": "123123",
      "attributes": {},
      "links": {
        "self": "/jobs/123123"
      }
    }
  ]
}
```

### POST /jobs/triggers/:trigger-id/launch

Launch a trigger manually given its ID and return the created job.

#### Request

```http
POST /jobs/triggers/123123/launch HTTP/1.1
Accept: application/vnd.api+json
```

#### Response

```json
{
  "data": {
    "type": "io.cozy.jobs",
    "id": "123123",
    "attributes": {
      "domain": "me.cozy.tools",
      "worker": "sendmail",
      "options": {},
      "state": "running",
      "queued_at": "2016-09-19T12:35:08Z",
      "started_at": "2016-09-19T12:35:08Z",
      "error": ""
    },
    "links": {
      "self": "/jobs/123123"
    }
  }
}
```

#### Permissions

To use this endpoint, an application needs a permission on the type
`io.cozy.triggers` for the verb `POST`.

### DELETE /jobs/triggers/:trigger-id

Delete a trigger given its ID.

#### Request

```http
DELETE /jobs/triggers/123123 HTTP/1.1
Accept: application/vnd.api+json
```

#### Permissions

To use this endpoint, an application needs a permission on the type
`io.cozy.triggers` for the verb `DELETE`.

Also, for konnector trigger, any application sharing the same doctype
permissions as the konnector launched by the trigger will be granted the
permissions to delete the trigger, even without permissions on the
`io.cozy.triggers` doctype.

### GET /jobs/triggers

Get the list of triggers.

Query parameters:

* `Worker`: to filter only triggers associated with a specific worker.

#### Request

```http
GET /jobs/triggers?Worker=konnector HTTP/1.1
Accept: application/vnd.api+json
```

#### Response

```json
{
  "data": [
    {
      "type": "io.cozy.triggers",
      "id": "123123",
      "attributes": {},
      "links": {
        "self": "/jobs/triggers/123123"
      }
    }
  ]
}
```

#### Permissions

To use this endpoint, an application needs a permission on the type
`io.cozy.triggers` for the verb `GET`. When used on a specific worker, the
permission can be specified on the `worker` field.

## Worker pool

The consuming side of the job queue is handled by a worker pool.

On a monolithic cozy-stack, the worker pool has a configurable fixed size of
workers. The default value is not yet determined. Each time a worker has
finished a job, it check the queue and based on the priority and the queued date
of the job, picks a new job to execute.

## Permissions

In order to prevent jobs from leaking informations between applications, we may
need to add filtering per applications: for instance one queue per applications.

We should also have an explicit check on the permissions of the applications
before launching a job scheduled by an application. For more information, refer
to our [permission document](./permissions).

## Multi-stack

When some instances are served by several stacks, the scheduling and running of
jobs can be distributed on the stacks. The synchronization is done via redis.

For scheduling, there is one important key in redis: `triggers`. It's a sorted
set. The members are the identifiants of the triggers (with the domain name),
and the score are the timestamp of the next time the trigger should occur.
During the short period of time where a trigger is processed, its key is moved
to `scheduling` (another sorted set). So, even if a stack crash during
processing a trigger, this trigger won't be lost.

For `@event` triggers, we don't use the same mechanism. Each stack has all the
triggers in memory and is responsible to trigger them for the events generated
by the HTTP requests of their API. They also publish them on redis: this pub/sub
is used for the realtime API.
