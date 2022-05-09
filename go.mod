module github.com/cozy/cozy-stack

go 1.15

require (
	github.com/Masterminds/semver/v3 v3.1.1
	github.com/andybalholm/brotli v1.0.4
	github.com/appleboy/go-fcm v0.1.5
	github.com/bradfitz/latlong v0.0.0-20170410180902-f3db6d0dff40
	github.com/cozy/goexif2 v0.0.0-20200819113101-00e1cc8cc9d3
	github.com/cozy/gomail v0.0.0-20170313100128-1395d9a6a6c0
	github.com/cozy/httpcache v0.0.0-20210224123405-3f334f841945
	github.com/cozy/prosemirror-go v0.5.0
	github.com/dhowden/tag v0.0.0-20201120070457-d52dcb253c63
	github.com/dustin/go-humanize v1.0.0
	github.com/go-redis/redis/v8 v8.11.5
	github.com/gofrs/uuid v4.1.0+incompatible
	github.com/golang-jwt/jwt/v4 v4.4.1
	github.com/golang/freetype v0.0.0-20170609003504-e2365dfdc4a0 // indirect
	github.com/golang/gddo v0.0.0-20210115222349-20d68f94ee1f
	github.com/goodsign/monday v1.0.0
	github.com/google/go-querystring v1.1.0
	github.com/google/gops v0.3.23
	github.com/gorilla/websocket v1.5.0
	github.com/h2non/filetype v1.1.3
	github.com/hashicorp/go-multierror v1.1.1
	github.com/howeyc/gopass v0.0.0-20210920133722-c8aef6fb66ef
	github.com/jonas-p/go-shp v0.1.1 // indirect
	github.com/justincampbell/bigduration v0.0.0-20160531141349-e45bf03c0666
	github.com/labstack/echo/v4 v4.7.2
	github.com/leonelquinteros/gotext v1.5.0
	github.com/mitchellh/mapstructure v1.5.0
	github.com/mssola/user_agent v0.5.3
	github.com/ncw/swift/v2 v2.0.1
	github.com/nightlyone/lockfile v1.0.0
	github.com/oschwald/maxminddb-golang v1.9.0
	github.com/pquerna/otp v1.3.0
	github.com/prometheus/client_golang v1.12.1
	github.com/robfig/cron/v3 v3.0.1
	github.com/sideshow/apns2 v0.23.0
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/afero v1.8.2
	github.com/spf13/cobra v1.4.0
	github.com/spf13/viper v1.10.1
	github.com/stretchr/testify v1.7.1
	github.com/ugorji/go/codec v1.2.7
	github.com/yuin/goldmark v1.4.12
	golang.org/x/crypto v0.0.0-20220507011949-2cf3adece122
	golang.org/x/image v0.0.0-20220413100746-70e8d0d3baa9
	golang.org/x/net v0.0.0-20220425223048-2871e0cb64e4
	golang.org/x/oauth2 v0.0.0-20220411215720-9780585627b5
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/text v0.3.7
	gopkg.in/alexcesaro/quotedprintable.v3 v3.0.0-20150716171945-2caba252f4dc // indirect
)

replace github.com/spf13/afero => github.com/cozy/afero v1.2.3
