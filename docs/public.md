[Table of contents](README.md#table-of-contents)

# Public routes

These routes are public: no authentication is required for them.

## Avatar

### GET /public/avatar

Returns an image chosen by the user as their avatar. If no image has been
chosen, a fallback will be used, depending of the `fallback` parameter in the
query-string:

- `default`: a default image that shows the Cozy Cloud logo, but it can be
  overriden by dynamic assets per context
- `initials`: a generated image with the initials of the owner's public name
- `404`: just a 404 - Not found error.
