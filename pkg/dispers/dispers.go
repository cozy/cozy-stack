package dispers

//import "github.com/cozy/cozy-stack/pkg/crypto" // to communicate

// supportedData specifies datsets on which you can train
var SupportedData = []string{
	"iris", "bank.label",
}

func DataSayHello() string {
	 return "Hello World ! I'm the Server DATA. I am going to pick up data and preprocess it !"
}
