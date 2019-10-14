module github.com/cozy/cozy-stack

go 1.12

require (
	github.com/Masterminds/semver v1.5.0
	github.com/appleboy/go-fcm v0.1.4
	github.com/bradfitz/latlong v0.0.0-20170410180902-f3db6d0dff40
	github.com/cozy/goexif2 v0.0.0-20190919162732-41879c76f051
	github.com/cozy/gomail v0.0.0-20170313100128-1395d9a6a6c0
	github.com/cozy/httpcache v0.0.0-20180914105234-d3dc4988de66
	github.com/dhowden/tag v0.0.0-20190519100835-db0c67e351b1
	github.com/dustin/go-humanize v1.0.0
	github.com/emersion/go-vcard v0.0.0-20190105225839-8856043f13c5
	github.com/go-redis/redis/v7 v7.0.0-beta.4
	github.com/gofrs/uuid v3.2.0+incompatible
	github.com/golang/freetype v0.0.0-20170609003504-e2365dfdc4a0 // indirect
	github.com/golang/gddo v0.0.0-20190904175337-72a348e765d2
	github.com/google/go-querystring v1.0.0
	github.com/google/gops v0.3.6
	github.com/gorilla/websocket v1.4.1
	github.com/h2non/filetype v1.0.10
	github.com/hashicorp/go-multierror v1.0.0
	github.com/howeyc/gopass v0.0.0-20190910152052-7cb4b85ec19c
	github.com/jonas-p/go-shp v0.1.1 // indirect
	github.com/justincampbell/bigduration v0.0.0-20160531141349-e45bf03c0666
	github.com/konsorten/go-windows-terminal-sequences v1.0.2 // indirect
	github.com/labstack/echo/v4 v4.1.6
	github.com/leonelquinteros/gotext v1.4.0
	github.com/magiconair/properties v1.8.1 // indirect
	github.com/mitchellh/mapstructure v1.1.2
	github.com/mssola/user_agent v0.5.0
	github.com/ncw/swift v1.0.49
	github.com/nightlyone/lockfile v0.0.0-20180618180623-0ad87eef1443
	github.com/oschwald/maxminddb-golang v1.5.0
	github.com/pelletier/go-toml v1.5.0 // indirect
	github.com/pkg/errors v0.8.1
	github.com/pquerna/otp v1.2.0
	github.com/prometheus/client_golang v1.1.0
	github.com/robfig/cron/v3 v3.0.0
	github.com/sideshow/apns2 v0.19.0
	github.com/sirupsen/logrus v1.4.2
	github.com/spf13/afero v1.2.2
	github.com/spf13/cobra v0.0.5
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/viper v1.4.0
	github.com/stretchr/testify v1.4.0
	github.com/ugorji/go/codec v1.1.7
	golang.org/x/crypto v0.0.0-20191002192127-34f69633bfdc
	golang.org/x/image v0.0.0-20191009234506-e7c1f5e7dbb8
	golang.org/x/net v0.0.0-20191011234655-491137f69257
	golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45
	google.golang.org/appengine v1.6.5 // indirect
	gopkg.in/alexcesaro/quotedprintable.v3 v3.0.0-20150716171945-2caba252f4dc // indirect
	gopkg.in/dgrijalva/jwt-go.v3 v3.2.0
)

replace github.com/spf13/afero => github.com/cozy/afero v1.2.3
