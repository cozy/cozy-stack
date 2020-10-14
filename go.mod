module github.com/cozy/cozy-stack

go 1.13

require (
	github.com/Masterminds/semver/v3 v3.1.0
	github.com/appleboy/go-fcm v0.1.5
	github.com/bradfitz/latlong v0.0.0-20170410180902-f3db6d0dff40
	github.com/cozy/goexif2 v0.0.0-20200819113101-00e1cc8cc9d3
	github.com/cozy/gomail v0.0.0-20170313100128-1395d9a6a6c0
	github.com/cozy/httpcache v0.0.0-20180914105234-d3dc4988de66
	github.com/cozy/prosemirror-go v0.4.6
	github.com/dhowden/tag v0.0.0-20200828214007-46e57f75dbfc
	github.com/dustin/go-humanize v1.0.0
	github.com/emersion/go-vcard v0.0.0-20200508080525-dd3110a24ec2
	github.com/go-redis/redis/v7 v7.4.0
	github.com/gofrs/uuid v3.3.0+incompatible
	github.com/golang/freetype v0.0.0-20170609003504-e2365dfdc4a0 // indirect
	github.com/golang/gddo v0.0.0-20200831202555-721e228c7686
	github.com/google/go-querystring v1.0.0
	github.com/google/gops v0.3.12
	github.com/gorilla/websocket v1.4.2
	github.com/h2non/filetype v1.1.0
	github.com/hashicorp/go-multierror v1.1.0
	github.com/howeyc/gopass v0.0.0-20190910152052-7cb4b85ec19c
	github.com/jonas-p/go-shp v0.1.1 // indirect
	github.com/justincampbell/bigduration v0.0.0-20160531141349-e45bf03c0666
	github.com/labstack/echo/v4 v4.1.6
	github.com/leonelquinteros/gotext v1.4.0
	github.com/mitchellh/mapstructure v1.3.3
	github.com/mssola/user_agent v0.5.2
	github.com/ncw/swift v1.0.52
	github.com/nightlyone/lockfile v1.0.0
	github.com/oschwald/maxminddb-golang v1.7.0
	github.com/pelletier/go-toml v1.5.0 // indirect
	github.com/pquerna/otp v1.2.0
	github.com/prometheus/client_golang v1.7.1
	github.com/robfig/cron/v3 v3.0.1
	github.com/sideshow/apns2 v0.20.0
	github.com/sirupsen/logrus v1.7.0
	github.com/spf13/afero v1.4.0
	github.com/spf13/cobra v1.0.0
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/viper v1.7.1
	github.com/stretchr/testify v1.6.1
	github.com/ugorji/go/codec v1.1.11
	golang.org/x/crypto v0.0.0-20201002170205-7f63de1d35b0
	golang.org/x/image v0.0.0-20200927104501-e162460cd6b5
	golang.org/x/net v0.0.0-20201010224723-4f7140c49acb
	golang.org/x/oauth2 v0.0.0-20200902213428-5d25da1a8d43
	golang.org/x/sync v0.0.0-20201008141435-b3e1573b7520
	gopkg.in/alexcesaro/quotedprintable.v3 v3.0.0-20150716171945-2caba252f4dc // indirect
	gopkg.in/dgrijalva/jwt-go.v3 v3.2.0
)

replace github.com/spf13/afero => github.com/cozy/afero v1.2.3
