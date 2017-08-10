[Table of contents](README.md#table-of-contents)

# Account types


## Google

Example creation of google account_type for the stack at .mycozy.cloud


http PUT localhost:5984/secrets%2Fio-cozy-accounts_types/google
grant_mode=authorization_code
redirect_uri="https://oauthcallback.mycozy.cloud/accounts/google/redirect"
token_endpoint="https://www.googleapis.com/oauth2/v4/token"
auth_endpoint="https://accounts.google.com/o/oauth2/v2/auth"
client_id=$CLIENT_ID
client_secret=$CLIENT_SECRET
