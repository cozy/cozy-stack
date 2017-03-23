[Table of contents](README.md#table-of-contents)

# Intents

- [Overview](#overview)
- [Glossary](#glossary)
- [Proposal](#proposal)
    - [1. Manifest](#1-manifest)
    - [2. Intent Start](##2-intent-start)
    - [3. Service Resolution](#3-service-resolution)
    - [4. Handshake](#4-handshake)
    - [5. Processing & Terminating](#5-processing--terminating)
- [Annex](#annex)
    - [Use cases](#use-cases)
    - [Bibliography & Prior Art](#bibliography--prior-art)
    - [Discarded Ideas](#discarded-ideas)

## Overview

A typical Cozy Cloud runs multiple applications, but most of these applications are focused on one task and interact with one particular type of data.

However, Cozy Cloud especially shines when data is combined across apps to provide an integrated experience. This is made difficult by the fact that apps have no dedicated back-end and that they have restricted access to documents.

This document outlines a proposal for apps to rely on each other to accomplish certain tasks and gain access to new documents, in a way that hopefully is neither painful for the users or the developers.

## Glossary

- **Intent**: Intents, sometimes also called Activities, is a pattern used in environments where multiple apps with different purposes coexist. The idea is that any app can express the need to do *something* that it can't do itself, and an app that *can* do it will take over from there.
- **Stack**: refers to [cozy-stack](https://github.com/cozy/cozy-stack/), the server-side part of the Cozy infrastructure.
- **Client**: the client is the application that *starts* an intent.
- **Service**: the service is the application that *handles* an intent started by a client.

## Proposal

In this proposal, services declare themselves through their manifest. When a clients starts an intent, the stack finds the appropriate service and help the two apps to create a communication channel between them. After the channel is ready, the intent is processed. These steps are detailed below.

### 1. Manifest

Every app can register itself as a potential handler for one or more intents. To do so, it must provide the following information for each intent it wishes to handle:

- `action`: A verb that describes what the service should do. The most common actions are `CREATE`, `EDIT`, `VIEW`, `PICK`, and `SHARE`. While we recommend using one of these verbs, the list is not exhaustive and may be extended by any app.
- `type`: One or more types of data on which the service knows how to operate the `action`. A `type` can be expressed as a [MIME type](https://en.wikipedia.org/wiki/Media_type) or a [Cozy Document Type](https://github.com/cozy/cozy-stack/blob/master/docs/data-system.md#typing). The application must have permissions for any Cozy Document Type listed here. You can also think of the `type` as the intent's subject.
- `href`: the relative url of the route designed to handle this intent.

These informations must be provided in the manifest of the application, inside the `intents` key.

Here is a very simple example:

```
"intents": [
    {
        "action": "PICK",
        "type": "io.cozy.files",
        "href": "/pick"
    }
]
```

Here is an example of an app that supports multiple data types:

```
"intents": [
    {
        "action": "PICK",
        "type": ["io.cozy.files", "image/*"],
        "href": "/pick"
    }
]
```

Finally, here is an example of an app that supports several intent types:

```
"intents": [
    {
        "action": "PICK",
        "type": ["io.cozy.files", "image/*"],
        "href": "/pick"
    },
    {
        "action": "VIEW",
        "type": "image/gif",
        "href": "/viewer"
    }
]
```

This information is stored by the stack when the application is installed.

### 2. Intent Start

Any app can start a new intent whenever it wants. When it does, the app becomes the *client*.

To start an intent, it must specify the following information:

- `action` : an action verb, which will be matched against the actions declared in services manifest files.
- `type` : a **single** data type, which will be matched against the types declared in services manifest files.

Based on the `type`, a data format to use for communications between the client and the service is inferred.

If `type` is a MIME type, than communications must be done using that MIME type.  
When `type` is a Cozy Document Type, the inferred MIME type is `application/json` and that format must be used for inter-apps communications.

There are also two optional fields that can be defined at the start of the intent:

- `data` : Any data that the client want to makes available to the service. This data must be represented in the inferred data format.
- `permissions` : When `type` is a Cozy Document Type and the client expects to receive one or more documents as part of the reply from the service, the `permissions` field allows the client to request permissions for these documents. `permissions` is a list of HTTP Verbs. Refer [to this section](https://github.com/cozy/cozy-stack/blob/master/docs/permissions.md#verbs) of the permission documentation for more information.

**Note**: if the intent's subject is a Cozy Doctype that holds references to other Cozy Documents (such as an album referencing photos or a playlist referencing music files), the permissions should be granted for the referenced documents too, whenever possible.

Here is an example of what the API could look like:

```js
// "Let the user pick a file"
cozy.startIntent('PICK', 'io.cozy.files')
.then(document => {
    // document is a JSON representation of the picked file
});

// "Create a contact, with some information already filled out"
cozy.startIntent('CREATE', 'io.cozy.contacts', {
    name: 'John Johnsons',
    tel: '+12345678'
    email: 'john@johnsons.com'
})
.then(document => {
    // document is a JSON representation of the contact that was created
});

// "Create a new note, and give me read-only access to it"
cozy.startIntent('CREATE', 'io.cozy.notes', {}, ['GET'])
.then(document => {
    // document is a JSON representation of the note that was created.
    // Additionally, this note can now be retrieved through the API since we have read access on it.
});

// "Create an event based on the provided data, and give me full access to it"
cozy.startIntent('CREATE', 'io.cozy.events', {
    date: '2017/06/24',
    title: 'Beach day'
}, ['ALL'])
.then(document => {
    // document is a JSON representation of the note that was created.
    // Additionally, this note can now be retrieved through the API since we have read access on it.
});

// "Crop this picture"
cozy.startIntent('EDIT', 'image/png', 'data:image/png;base64,iVBORw...'})
.then(image => {
    //image is the edited version of the image provided above.
})
```

### 3. Service Resolution

The service resolution is the phase where a service is chosen to handle an intent. This phase is handled by the stack.

After the client has started it's intent, it sends the `action` and `type` to the stack. The stack will then traverse the list of installed apps and find all the apps that can handle that specific combination of `action` and `type`.

The stack then stores the information for that intent:

- Client URL
- Service URL
- `action`
- `type`

Finally, it replies to the client's request with the service's URL.

#### Service choice

If more than one service match the `action` and `type`, the stack returns a special URL instead of the service's URL. This URL will display a page (the "choice page") where the user is invited to chose between one of the available services, and where he/she can persist that choice as a preference.

After a service is selected, this choice page will redirect itself to the chosen service's URL and the flow resumes as normal.
**Note: this part needs to be discussed further.**

The user may decide to abort the intent before picking a service. If that is the case, the choice page will need to inform the client that the intent was aborted.

#### No available service

If no service is available to handle an intent, the stack returns an error to the client.

At a later phase of this project, the stack may traverse the applications registered in a store to find suitable services, and prompt the user to install one.

### 4. Handshake

The next phase consist of the client and the service establishing a communication channel between them. The communication will be done using the [window.postMessage](https://developer.mozilla.org/fr/docs/Web/API/Window/postMessage) API.

#### Service Initialization

When the client receives the service URL from the stack, it starts to listen for messages coming from that URL. Once the listener is started, it opens an iframe that points to the service's URL.

#### Service to Client

At this point, the service app is opened on the view that it provided in the `href` part of it's manifest.

The service queries the stack to find out information about the intent that it should handle. This includes the client's URL, the `action` and the `type`.

It then starts to listen for messages coming from the client's URL. Eventually, it sends a message to the client, as a mean to inform it that the service is now ready.

#### Client to Service

After the client receives the "ready" message from the service, it sends a message to the service acknowledging the "ready" state.

Along with this message, it should send the `data` and `permissions` if they were provided at  the start of the intent.

### 5. Processing & Terminating

After this handshake, there is a confirmed communication channel between the client and the service, and the service knows what it has to do. This is the phase where the user interacts with the service.

If the service is going to grant extra permissions to the client app, it is strongly recommended to make this clear to the user.

When the service has finished his task, it sends a "completed" message to the client. Permissions extensions should have been done before that. Along with the completed message, the service should send any relevant data.

After the client receives a "completed" message, it can close the service's iframe and resume operations as usual.

If, for whatever reason, the service can not fulfill the intent, it can send an "error" message to the client.
When the client receives an "error" message, the intent is aborted and the iframe can be closed.

## Annex

### Use Cases

Here is a non exhaustive list of situations that *may* use intents:

- Configure a new connector account
- Share a photo album via email
- Add an attachment to an email
- Attach a note to a contact
- Create a contact based on an email
- Save an attachment received in an email
- Create a birthday event in a calendar, based on a contact
- Create an event based on an email's content
- Create an event based on an ICS file
- Chose an avatar for a contact
- Open a file from the file browser (music, PDF, image, ...)
- Attach a receipt to an expense
- Provide a tip for another application

### Bibliography & Prior Art

- Prior art: https://forum.cozy.io/t/cozy-tech-topic-inter-app-communication-architecture/2287
- Web intents: http://webintents.org/
- WebActivities: https://wiki.mozilla.org/WebAPI/WebActivities and https://developer.mozilla.org/en-US/docs/Archive/Firefox_OS/API/Web_Activities
- Siri Intents: https://developer.apple.com/library/content/documentation/Intents/Conceptual/SiriIntegrationGuide/ResolvingandHandlingIntents.html#//apple_ref/doc/uid/TP40016875-CH5-SW1
- Android Intents: https://developer.android.com/reference/android/content/Intent.html
- iOS extensions: https://developer.apple.com/library/content/documentation/General/Conceptual/ExtensibilityPG/index.html

### Discarded Ideas

#### Disposition

Some specifications include a `disposition` field in the manifests, that gives a hint to the client about how to display the service (inlined or in a new window).
Since we were unable to find a use case where a new window is required, we decided not to include the `disposition` in this specification. It might be added later if the need arises.

#### Client / Server architecture

Instead of letting the two applications communicate with each other, they could be made to talk through the stack.
This would be useful as the stack could act as a middleware, transforming data on the fly or granting permissions where appropriate.

However, this approach has also severe drawbacks, notably the fact that the stack must hold a copy of all the data that the apps want to transfer (which can include large files). It also makes it significantly harder for the client to know when the intent has been processed.

#### Data representation

The client could explicitly request a data format to be used in the communication. However, this idea was abandoned because the format is *always* json, except when then intent's `type` is a MIME type, in which case the data format *also* uses this type.
