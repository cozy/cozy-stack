[Table of contents](README.md#table-of-contents)

# AI for personal data

## Introduction

AI can be used for interacting with the personal data of a Cozy. This is
currently an experimental feature. Retrieval-Augmented Generation (RAG) is
a classical pattern in the AI world. Here, it is specific to each Cozy.

[OpenWebUI](https://openwebui.com/) has been integrated this way:

![Architecture with OpenWebUI](diagrams/openwebui.svg)

## Indexation

First of all, OpenWebUI and the RAG must be installed with their dependencies.
It is not mandatory to install them on the same servers as the cozy-stack. And
the URL of RAG must be filled in cozy-stack configuration file (in
`external_indexers`).

For the moment, the feature is experimental, and a trigger must be created
manually on the Cozy:

```sh
$ COZY=cozy.localhost:8080
$ TOKEN=$(cozy-stack instances token-cli $COZY io.cozy.triggers)
$ curl "http://${COZY}/jobs/triggers" -H "Authorization: Bearer $TOKEN" -d '{ "data": { "attributes": { "type": "@event", "arguments": "io.cozy.files", "debounce": "1m", "worker": "index", "message": {"doctype": "io.cozy.files"} } } }'
```

It can also be a good idea to start a first indexation with:

```sh
$ cozy-stack triggers launch --domain $COZY $TRIGGER_ID
```

In practice, when files are uploaded/modified/deleted, the trigger will create
a job for the index worker (with debounce). The index worker will look at the
changed feed, and will call the RAG for each entry in the changes feed.


## Chat

When a user starts a chat in OpenWebUI, their prompts are sent to the RAG that
can use the vector database to find relevant documents (technically, only some
parts of the documents called chunks). Those documents are sent back to
LibreChat that can be added to the prompt, so that the LLM can use them as a
context when answering.

### GET /ai/open

This route returns the parameters to open an iframe with OpenWebUI. It creates
an account on the OpenWebUI chat server if needed, creates a token an returns
the configured URL of the OpenWebUI chat server.

### Request

```http
GET /ai/open HTTP/1.1
```

### Response

```http
HTTP/1.1 200 OK
Content-Type: application/vnd.api+json
```

```json
{
  "data": {
    "type": "io.cozy.ai.url",
    "id": "eeec30e8-02d9-4988-80a5-acdb238f8d10",
    "attributes": {
      "url": "https://openwebui.example.org/",
      "token:": "eyJh...Md4o"
    }
  }
}
```

The webapp can then creates an iframe:

```html
<iframe src="https://openwebui.example.org/" data-token="eyJh...Md4o"></iframe>
```

and inside the iframe loads the token with:

```js
localStorage.token = window.frameElement.dataset.token
```
