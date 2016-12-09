Cozy Jobs
=========

Jobs are designed to represent asynchronous tasks that your cozy can execute. These tasks can be scheduled in advance, recurring or suddnn and provide various services.

At the time, we do not provide "generic" and programmable tasks. This list of available workers is part of the defined API.

The job queue can be either local to the cozy-stack when used as a monolitic instance (self-hosted for instance) or distributed via a redis server for distributed infrastructures.

This doc introduces two cozy types:

  - `io.cozy.jobs` for jobs
  - `io.cozy.launchers` for launchers


Launchers
---------

Jobs can be launched by three types of launchers:

  - `@cron` to schedule recurring jobs scheduled at specific times
  - `@interval` to schedule periodic jobs executed at a given fix interval
  - `@event` to launch a job after a change in the cozy

These three launchers have specific syntaxes to describe when jobs should be scheduled. See below for more informations.

Jobs can also be queued up programatically, without the help of a specific launcher directly via the `/jobs/queue` API. In this case the `launcher` name is empty.

### `@cron` syntax

In order to schedule recurring jobs, the `cron` launcher has the syntax using six fields:

```
Field name   | Mandatory? | Allowed values  | Allowed special characters
----------   | ---------- | --------------  | --------------------------
Seconds      | Yes        | 0-59            | * / , -
Minutes      | Yes        | 0-59            | * / , -
Hours        | Yes        | 0-23            | * / , -
Day of month | Yes        | 1-31            | * / , - ?
Month        | Yes        | 1-12 or JAN-DEC | * / , -
Day of week  | Yes        | 0-6 or SUN-SAT  | * / , - ?
------------------------------------------------------------------------

Asterisk ( * )

The asterisk indicates that the cron expression will match for all values of
the field; e.g., using an asterisk in the 5th field (month) would indicate
every month.

Slash ( / )

Slashes are used to describe increments of ranges. For example 3-59/15 in the
1st field (minutes) would indicate the 3rd minute of the hour and every 15
minutes thereafter. The form "*\/..." is equivalent to the form "first-
last/...", that is, an increment over the largest possible range of the field.
The form "N/..." is accepted as meaning "N-MAX/...", that is, starting at N,
use the increment until the end of that specific range. It does not wrap
around.

Comma ( , )

Commas are used to separate items of a list. For example, using "MON,WED,FRI"
in the 5th field (day of week) would mean Mondays, Wednesdays and Fridays.

Hyphen ( - )

Hyphens are used to define ranges. For example, 9-17 would indicate every hour
between 9am and 5pm inclusive.

Question mark ( ? )

Question mark may be used instead of '*' for leaving either day-of-month or
day-of-week blank.
```

To schedule jobs given an interval:

Examples:

```
@cron 0 0 0 1 1 * # Run once a year, midnight, Jan. 1st
@cron 0 0 0 1 1 * # Run once a year, midnight, Jan. 1st
@cron 0 0 0 1 * * # Run once a month, midnight, first of month
@cron 0 0 0 * * 0 # Run once a week, midnight on Sunday
@cron 0 0 0 * * * # Run once a day, midnight
@cron 0 0 * * * * # Run once an hour, beginning of hour
```


### `@interval` syntax

The `@interval` launcher uses the same syntax as golang's `time.ParseDuration` (only supporting time units above seconds):

```
A duration string is a possibly signed sequence of decimal numbers, each with
optional fraction and a unit suffix, such as "300ms", "1.5h" or "2h45m". Valid
time units are "s", "m", "h".
```

Examples

```
@interval 1.5h   # schedules every 1 and a half hours
@interval 30m10s # schedules every 30 minutes and 10 seconds
```


### `@event` syntax

The `@event` syntax is not determined yet. Its main purpose will be to describe job scheduling after a filesystem or database modification.


### Launcher rate limitation

Every launcher has a rate limitation policy to prevent launchers from spawning too many jobs at the same time. The specific rules are not yet decided, but they should work along these two properties:

#### Parallel limitations

Each launcher will have a limited number of workers it is allowed to queue in parallel.

#### Back pressure

Each launcher should have a backpressure policy to drop job spawning when not necessary. For instance: 

* *throttling policy* (aka *debouncing*) to drop job actions given timings parameters. ie. a job scheduled after contact updates should only be triggered once after several contacts are updated in a given time lapse, or should be scheduled when the updates stopped for a given time
* *side effect limitation* in the case of an `@event` launcher, a job doing an external API call should not be spawned if another one is already running for another close event
* *full queue* when no worker is available, or the queue has too many elements, it can decide to drop the job action (given some informations)


Error Handling
--------------

Jobs can fail to execute their task. We have two ways to parametrize such cases.

### Retry

A retry count can be optionnaly specified to ask the worker to re-execute the task if it has failed.

Each retry is executed after a configurable delay. The try count is part of the attributes of the job. Also, each occuring error is kept in the `errors` field containing all the errors that may have happened.

### Timeout

A worker may never end. To prevent this, a configurable timeout value is specified with the job.

If a job does not end after the specified amount of time, it will be aborted. A timeout is just like another error from the worker and can provoke a retry if specified.

### Defaults

By default, jobs are paramterized with a maximum of 3 tries with 1 minute timeout.

These defaults may vary given the workload of the workers.


Jobs API
--------

Example and description of the attributes of a `io.cozy.jobs`:

```js
{
  "worker": "sendmail",    // worker type name
  "worker_id": "123123",   // worker id, if any
  "launcher": "@cron",     // "@cron", "@interval", "@event" or ""
  "launcher_id": "1234",   // launcher id, if any
  "options": {
    "priority": 3,         // priority from 1 to 100, higher number is higher priority
    "timeout": 60,         // timeout value in seconds
    "retry_max_count": 3,  // maximum number of retry
    "retry_delay": 10,     // retry delay in seconds between each try
    "arguments": {},        // arguments message
  },
  "state": "running",      // queued, running, errored
  "worker_id": "123456",   // or unknown if in the queue
  "try_count": 1,          // number of time the job has been executed.
                           // increased at the start of new execution
  "queued_at": "2016-09-19T12:35:08Z",  // time of the queuing
  "started_at": "2016-09-19T12:35:08Z", // time of first execution
  "errors": [],            // list of errors
  "output": {},            // output of the worker, if any
}
```

Example and description of a job creation options — as you can see, the options are replicated in the `io.cozy.jobs` attributes:

```js
{
  "priority": 3,         // priority from 1 to 100
  "timeout": 60,         // timeout value in seconds
  "retry_max_count": 3,  // maximum number of retry
  "retry_delay": 10,     // retry delay in seconds between each try
  "arguments": {},        // arguments message
}
```


### GET /jobs/:job-id

Get a job description given its ID.

#### Status codes

* 200 OK, when the job is found its description written
* 404 Not Found, when the job has not been found (it either never existed or is finished)

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
    "rev": "1-12334",
    "attributes": {
      "worker": "sendmail",
      "worker_id": "123123",
      "launcher": "@cron",
      "launcher_id": "4321",
      "options": {
        "priority": 3,
        "timeout": 60,
        "retry_max_count": 3,
        "retry_delay": 10,
        "arguments": {},
      },
      "state": "running",
      "worker_id": "123456",
      "try_count": 1,
      "queued_at": "2016-09-19T12:35:08Z",
      "started_at": "2016-09-19T12:35:08Z",
      "errors": [],
      "output": {},
    }
  }
}
```


### POST /jobs/queue/:worker-name

Enqueue programmatically a new job.

#### Request

```http
POST /jobs/queue/sendmail HTTP/1.1
Accept: application/vnd.api+json

{
  "priority": 3,
  "timeout": 60,
  "retry_max_count": 3,
  "retry_delay": 10,
  "arguments": {},
}
```

#### Response

```json
{
  "data": {
    "type": "io.cozy.jobs",
    "id": "123123",
    "rev": "1-12334",
    "attributes": {
      "worker": "sendmail",
      "worker_id": "123123",
      "launcher": "@cron",
      "launcher_id": "4321",
      "options": {
        "priority": 3,
        "timeout": 60,
        "retry_max_count": 3,
        "retry_delay": 10,
        "arguments": {},
      },
      "state": "running",
      "worker_id": "123456",
      "try_count": 1,
      "queued_at": "2016-09-19T12:35:08Z",
      "started_at": "2016-09-19T12:35:08Z",
      "errors": [],
      "output": {},
    },
    "links": {
      "self": "/jobs/123123"
    }
  }
}
```


### GET /jobs/queue

List the jobs in the queue.

#### Request

```http
GET /jobs/queue HTTP/1.1
Accept: application/vnd.api+json
```

#### Response

```json
{
  "links": {
    "next": "/jobs/queue?page[cursor]=123123"
  },
  "data": [
    {
      "type": "io.cozy.jobs",
      "id": "123123",
      "rev": "1-12334",
      "attributes": {},
      "links": {
        "self": "/jobs/123123"
      }
    },
  ]
}
```


### POST /jobs/launchers/:worker-name

Add a launcher of the worker. See [launchers' descriptions](#launchers) to see the types of launcher and their arguments syntax.

#### Request

```http
POST /jobs/launchers/sendmail HTTP/1.1
Accept: application/vnd.api+json

{
  "type": "@interval",
  "arguments": "30m10s",
  "options": {
    "priority": 3,
    "timeout": 60,
    "retry_max_count": 3,
    "retry_delay": 10,
    "arguments": {},
  }
}
```

#### Response

```json
{
  "data": {
    "type": "io.cozy.launchers",
    "id": "123123",
    "rev": "1-12334",
    "attributes": {
      "type": "@interval",
      "arguments": "30m10s",
      "worker": "sendmail",
      "options": {
        "priority": 3,
        "timeout": 60,
        "retry_max_count": 3,
        "retry_delay": 10,
        "arguments": {},
      }
    },
    "links": {
      "self": "/jobs/launchers/123123"
    }
  }
}
```


### GET /jobs/launchers/:launcher-id

Get a launcher informations given its ID.

#### Request

```http
GET /jobs/launchers/123123 HTTP/1.1
Accept: application/vnd.api+json
```

#### Response

```json
{
  "data": {
    "type": "io.cozy.launchers",
    "id": "123123",
    "rev": "1-12334",
    "attributes": {
      "type": "@interval",
      "arguments": "30m10s",
      "worker": "sendmail",
      "options": {
        "priority": 3,
        "timeout": 60,
        "retry_max_count": 3,
        "retry_delay": 10,
        "arguments": {},
      }
    },
    "links": {
      "self": "/jobs/launchers/123123"
    }
  }
}
```


### GET /jobs/launchers/

Get the list of launchers.

#### Request

```http
GET /jobs/launchers/ HTTP/1.1
Accept: application/vnd.api+json
```

#### Response

```json
{
  "data": [
    {
      "type": "io.cozy.launchers",
      "id": "123123",
      "rev": "1-12334",
      "attributes": {},
      "links": {
        "self": "/jobs/launchers/123123"
      }
    },
  ]
}
```


### DELETE /jobs/launchers/:launcher-id

Delete a launcher given its ID.

#### Request

```http
DELETE /jobs/launchers/123123 HTTP/1.1
Accept: application/vnd.api+json
```

#### Status codes

* 204 No Content, when the launcher has been successfully removed
* 404 Not Found, when the launcher does not exist


Worker pool
-----------

The consuming side of the job queue is handled by a worker pool.

On a monolithic cozy-stack, the worker pool has a parametrizable fixed size of workers. The default value is not yet determined. Each time a worker has finished a job, it check the queue and based on the priority and the queued date of the job, picks a new job to execute.


Permissions
-----------

In order to prevent jobs from leaking informations between applications, we may need to add filtering per applications: for instance one queue per applications.

We should also have an explicit check on the permissions of the applications before launching a job scheduled by an application. For more information, refer to our [permission document](./permissions).
