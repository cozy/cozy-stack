package build

const (
	// ModeDev is the development release value
	ModeDev = "development"
	// ModeProd is the production release value
	ModeProd = "production"
)

var (
	// Version of the release (see scripts/build.sh script)
	Version string
	// BuildTime is ISO-8601 UTC string representation of the time of
	// the build
	BuildTime string
	// BuildMode is the build mode of the release. Should be either
	// production or development.
	BuildMode = ModeDev
)

// IsDevRelease returns whether or not the binary is a development
// release
func IsDevRelease() bool {
	return BuildMode == ModeDev
}
