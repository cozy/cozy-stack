[Table of contents](README.md#table-of-contents)

# Proxy for remote data/API

The client side applications in Cozy are constrained and cannot speak with
external websites to avoid leaking personal data. Technically, it is made with
the Content Security Policy. These rules are very strict and it would be a pity
to now allow a client side app to load public informations from a source like
Wikipedia. Our proposal is to make client side apps able to query external
websites of their choices, but these requests will be made via the cozy-stack
(as a proxy) and will be logged to check later that no personal data was leaked
unintentionally. We can see the data loaded from the external website as a
document with a doctype: it's just not a doctype local to our cozy, but a remote
one.

A client side app can request data from external websites by doing these three
steps:

1. Declaring a remote doctype and its requests
2. Declaring permissions on these doctypes in the manifest
3. Calling the `/remote` API of cozy-stack

## Declaring a remote doctype

Doctypes are formalized in this repository:
[github.com/cozy/cozy-doctypes](https://github.com/cozy/cozy-doctypes). Each
doctype has its own directory inside the repository. For a remote doctype, it
will include a file called `request` that will describe how the cozy-stack will
request the external website.

Let's take an example:

```
$ tree cozy-doctypes
cozy-doctypes
├── [...]
├── org.wikidata.entity
│   └── request
└── org.wikidata.search
    └── request

$ cat cozy-doctypes/org.wikidata.entity/request
GET https://www.wikidata.org/wiki/Special:EntityData/{{entity}}.json
Accept: application/json

$ cat cozy-doctypes/org.wikidata.search/request
GET https://www.wikidata.org/w/api.php?action=wbsearchentities&search={{q}}&language=en&format=json
```

Here, we have two remote doctypes. Each one has a request defined for it.

The format for the request file is:

-   the verb and the URL on the first line
-   then some lines that describe the HTTP headers
-   then a blank line and the body if the request is a POST

For the path, the query-string, the headers, and the body, it's possible to have
some dynamic part by using `{{`, a variable name, and `}}`.

Some templating helpers are available to escape specific variables using `{{`
function name - space - variable name `}}`. These helpers are only available for
the body part of the template.

Available helpers:

-   `json`: for json parts (`{ "key": "{{json val}}" }`)
-   `html`: for html parts (`<p>{{html val}}</p>`)
-   `query`: for query parameter of a url (`http://foobar.com?q={{query q}}`)
-   `path`: for path component of a url (`http://foobar.com/{{path p}}`)

Values injected in the URL are automatically URI-escaped based on the part they
are included in: namely as a query parameter or as a path component.

**Note**: by default, the User-Agent is set to a default value ("cozy-stack" and
a version number). It can be overriden in the request description.

Example:

```
GET https://foobar.com/{{path}}?q={{query}}
Content-Type: {{contentType}}

{
  "key": "{{json value}}",
  "url": "http://anotherurl.com/{{path anotherPath}}?q={{query anotherQuery}}",
}
```

## Declaring permissions

Nothing special here. The client side app must declare that it will use these
doctypes in its manifest, like for other doctypes:

```json
{
    "...": "...",
    "permissions": {
        "search": {
            "description": "Required for searching on wikidata",
            "type": "org.wikidata.search"
        },
        "entity": {
            "description": "Required for getting more informations about an entity on wikidata",
            "type": "org.wikidata.entity"
        }
    }
}
```

## Calling the remote API

### GET/POST `/remote/:doctype`

The client side app must use the same verb as the defined request (`GET` in our
two previous examples). It can use the query-string for GET, and a JSON body for
POST, to give values for the variables.

Example:

```http
GET /remote/org.wikidata.search?q=Douglas+Adams HTTP/1.1
Host: alice.cozy.tools
```

It is possible to send some extra informations, to make it easier to understand
the request. If no variable in the request matches it, it won't be send to the
remote website.

Example:

```http
POST /remote/org.example HTTP/1.1
Host: alice.cozy.tools
Content-Type: application/json
```

```json
{
    "query": "Qbhtynf Nqnzf",
    "comment": "query is rot13 for Douglas Adams"
}
```

**Note**: currently, only the response with a content-type that is an image,
JSON or XML are accepted. Other content-types are blocked the time to evaluate
if they are useful and their security implication (javascript is probably not
something we want to allow).

### GET `/remote/assets/:asset-name`

The client application can fetch a list of predefined assets via this route. The
resources available are defined in the configuration file.

Example:

```http
GET /remote/assets/bank HTTP/1.1
Host: alice.cozy.tools
```

## Logs

The requests are logged as the `io.cozy.remote.requests` doctype, with the
doctype asked, the parameter (even those that have not been used, like `comment`
in the previous example), and the application that has made the request.

## For developers

If you are a developer and you want to use a new remote doctype, it can be
difficult to first make it available in the github.com/cozy/cozy-doctypes
repository and only then test it. So, the cozy-stack serve command will have a
`--doctypes` option to gives a local directory with the doctypes. You can fork
the repository, clone it, work on a new doctype inside, test it locally, and
when OK, make a pull request for it.
