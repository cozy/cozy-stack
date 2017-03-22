[Table of contents](README.md#table-of-contents)

# Intents

## What is an intent?

An intent is a way for client-side apps to use features of other apps. When a
client-side app is installed, its manifest lists the features that are exposed
to other apps. When a client-side app is running, it can ask the cozy-stack
which apps can serve this "intent". The other app will be loaded in an iframe,
where the user can interact. And the iframe will be closed when the intent is
done or cancelled.

For example, the files application can ask another app to play a music or a
video. Or, the contacts can ask the user to select a file in the files app to
be used as the avatar of a given contact.
