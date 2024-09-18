[Table of contents](README.md#table-of-contents)

# AI for personal data

## Introduction

AI can be used for interacting with the personal data of a Cozy. This is
currently an experimental feature. Retrieval-Augmented Generation (RAG) is
a classical pattern in the AI world. Here, it is specific to each Cozy.

![Architecture with a RAG server](diagrams/ai.svg)

## Indexation

First of all, the RAG server must be installed with its dependencies. It is
not mandatory to install them on the same servers as the cozy-stack. And the
URL of RAG must be filled in cozy-stack configuration file (in `rag`).

For the moment, the feature is experimental, and a trigger must be created
manually on the Cozy:

```sh
$ COZY=cozy.localhost:8080
$ TOKEN=$(cozy-stack instances token-cli $COZY io.cozy.triggers)
$ curl "http://${COZY}/jobs/triggers" -H "Authorization: Bearer $TOKEN" -d '{ "data": { "attributes": { "type": "@event", "arguments": "io.cozy.files", "debounce": "1m", "worker": "rag-index", "message": {"doctype": "io.cozy.files"} } } }'
```

It can also be a good idea to start a first indexation with:

```sh
$ cozy-stack triggers launch --domain $COZY $TRIGGER_ID
```

In practice, when files are uploaded/modified/deleted, the trigger will create
a job for the index worker (with debounce). The index worker will look at the
changed feed, and will call the RAG for each entry in the changes feed.

## Chat

When a user starts a chat, their prompts are sent to the RAG that can use the
vector database to find relevant documents (technically, only some parts of
the documents called chunks). Those documents are added to the prompt, so
that the LLM can use them as a context when answering.
