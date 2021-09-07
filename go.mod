module github.com/cozy/cozy-stack

go 1.13

require (
	github.com/Masterminds/semver/v3 v3.1.1
	github.com/andybalholm/brotli v1.0.3
	github.com/appleboy/go-fcm v0.1.5
	github.com/bradfitz/latlong v0.0.0-20170410180902-f3db6d0dff40
	github.com/cozy/goexif2 v0.0.0-20200819113101-00e1cc8cc9d3
	github.com/cozy/gomail v0.0.0-20170313100128-1395d9a6a6c0
	github.com/cozy/httpcache v0.0.0-20210224123405-3f334f841945
	github.com/cozy/prosemirror-go v0.4.11
	github.com/dgrijalva/jwt-go v3.2.0+incompatible // indirect
	github.com/dhowden/tag v0.0.0-20201120070457-d52dcb253c63
	github.com/dustin/go-humanize v1.0.0
	github.com/go-redis/redis/v7 v7.4.1
	github.com/gofrs/uuid v3.4.0+incompatible
	github.com/golang/freetype v0.0.0-20170609003504-e2365dfdc4a0 // indirect
	github.com/golang/gddo v0.0.0-20210115222349-20d68f94ee1f
	github.com/goodsign/monday v1.0.0
	github.com/google/go-querystring v1.1.0
	github.com/google/gops v0.3.19
	github.com/gorilla/websocket v1.4.2
	github.com/h2non/filetype v1.1.1
	github.com/hashicorp/go-multierror v1.1.1
	github.com/howeyc/gopass v0.0.0-20190910152052-7cb4b85ec19c
	github.com/jonas-p/go-shp v0.1.1 // indirect
	github.com/justincampbell/bigduration v0.0.0-20160531141349-e45bf03c0666
	github.com/labstack/echo/v4 v4.5.0
	github.com/leonelquinteros/gotext v1.5.0
	github.com/mitchellh/mapstructure v1.4.1
	github.com/mssola/user_agent v0.5.3
	github.com/ncw/swift v1.0.53
	github.com/nightlyone/lockfile v1.0.0
	github.com/onsi/ginkgo v1.15.0 // indirect
	github.com/onsi/gomega v1.10.5 // indirect
	github.com/oschwald/maxminddb-golang v1.8.0
	github.com/pquerna/otp v1.3.0
	github.com/prometheus/client_golang v1.11.0
	github.com/robfig/cron/v3 v3.0.1
	github.com/sideshow/apns2 v0.20.0
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/afero v1.6.0
	github.com/spf13/cobra v1.2.1
	github.com/spf13/viper v1.8.1
	github.com/stretchr/testify v1.7.0
	github.com/ugorji/go/codec v1.2.6
	golang.org/x/crypto v0.0.0-20210813211128-0a44fdfbc16e
	golang.org/x/image v0.0.0-20210628002857-a66eb6448b8d
	golang.org/x/net v0.0.0-20210813160813-60bc85c4be6d
	golang.org/x/oauth2 v0.0.0-20210810183815-faf39c7919d5
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	gopkg.in/alexcesaro/quotedprintable.v3 v3.0.0-20150716171945-2caba252f4dc // indirect
	gopkg.in/dgrijalva/jwt-go.v3 v3.2.0
)

replace github.com/spf13/afero => github.com/cozy/afero v1.2.3
