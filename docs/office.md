[Table of contents](README.md#table-of-contents)

# Collaborative edition of Office documents

## Diagrams

### Opening a document with OnlyOffice

Reference: https://api.onlyoffice.com/editors/open

![Opening a document with OnlyOffice](diagrams/onlyoffice-open.png)

1. The browser makes a request `GET /office/open` to know the address of the OnlyOffice server
2. The browser makes several HTTP requests to the Document Server
    1. Fetch the `api.js` script
    2. Open a websocket connection
    3. Send a command to load the office document
3. The document server makes a request to the [callback URL](https://api.onlyoffice.com/editors/callback#status-1) with `status=1`
4. The document server asks the file converter to load the document (via RabbitMQ?)
5. The file converter loads the file content from the stack

## Routes
