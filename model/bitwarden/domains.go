package bitwarden

// GlobalEquivalentDomainsType is an enum for global domain identifiers.
type GlobalEquivalentDomainsType int

// The list of all the global domain identifiers
// https://github.com/bitwarden/server/blob/master/src/Core/Enums/GlobalEquivalentDomainsType.cs
const (
	Google        GlobalEquivalentDomainsType = 0
	Apple         GlobalEquivalentDomainsType = 1
	Ameritrade    GlobalEquivalentDomainsType = 2
	BoA           GlobalEquivalentDomainsType = 3
	Sprint        GlobalEquivalentDomainsType = 4
	WellsFargo    GlobalEquivalentDomainsType = 5
	Merrill       GlobalEquivalentDomainsType = 6
	Citi          GlobalEquivalentDomainsType = 7
	Cnet          GlobalEquivalentDomainsType = 8
	Gap           GlobalEquivalentDomainsType = 9
	Microsoft     GlobalEquivalentDomainsType = 10
	United        GlobalEquivalentDomainsType = 11
	Yahoo         GlobalEquivalentDomainsType = 12
	Zonelabs      GlobalEquivalentDomainsType = 13
	PayPal        GlobalEquivalentDomainsType = 14
	Avon          GlobalEquivalentDomainsType = 15
	Diapers       GlobalEquivalentDomainsType = 16
	Contacts      GlobalEquivalentDomainsType = 17
	Amazon        GlobalEquivalentDomainsType = 18
	Cox           GlobalEquivalentDomainsType = 19
	Norton        GlobalEquivalentDomainsType = 20
	Verizon       GlobalEquivalentDomainsType = 21
	Buy           GlobalEquivalentDomainsType = 22
	Sirius        GlobalEquivalentDomainsType = 23
	Ea            GlobalEquivalentDomainsType = 24
	Basecamp      GlobalEquivalentDomainsType = 25
	Steam         GlobalEquivalentDomainsType = 26
	Chart         GlobalEquivalentDomainsType = 27
	Gotomeeting   GlobalEquivalentDomainsType = 28
	Gogo          GlobalEquivalentDomainsType = 29
	Oracle        GlobalEquivalentDomainsType = 30
	Discover      GlobalEquivalentDomainsType = 31
	Dcu           GlobalEquivalentDomainsType = 32
	Healthcare    GlobalEquivalentDomainsType = 33
	Pepco         GlobalEquivalentDomainsType = 34
	Century21     GlobalEquivalentDomainsType = 35
	Comcast       GlobalEquivalentDomainsType = 36
	Cricket       GlobalEquivalentDomainsType = 37
	Mtb           GlobalEquivalentDomainsType = 38
	Dropbox       GlobalEquivalentDomainsType = 39
	Snapfish      GlobalEquivalentDomainsType = 40
	Alibaba       GlobalEquivalentDomainsType = 41
	Playstation   GlobalEquivalentDomainsType = 42
	Mercado       GlobalEquivalentDomainsType = 43
	Zendesk       GlobalEquivalentDomainsType = 44
	Autodesk      GlobalEquivalentDomainsType = 45
	RailNation    GlobalEquivalentDomainsType = 46
	Wpcu          GlobalEquivalentDomainsType = 47
	Mathletics    GlobalEquivalentDomainsType = 48
	Discountbank  GlobalEquivalentDomainsType = 49
	Mi            GlobalEquivalentDomainsType = 50
	Facebook      GlobalEquivalentDomainsType = 51
	Postepay      GlobalEquivalentDomainsType = 52
	Skysports     GlobalEquivalentDomainsType = 53
	Disney        GlobalEquivalentDomainsType = 54
	Pokemon       GlobalEquivalentDomainsType = 55
	Uv            GlobalEquivalentDomainsType = 56
	Yahavo        GlobalEquivalentDomainsType = 57
	Mdsol         GlobalEquivalentDomainsType = 58
	Sears         GlobalEquivalentDomainsType = 59
	Xiami         GlobalEquivalentDomainsType = 60
	Belkin        GlobalEquivalentDomainsType = 61
	Turbotax      GlobalEquivalentDomainsType = 62
	Shopify       GlobalEquivalentDomainsType = 63
	Ebay          GlobalEquivalentDomainsType = 64
	Techdata      GlobalEquivalentDomainsType = 65
	Schwab        GlobalEquivalentDomainsType = 66
	Mozilla       GlobalEquivalentDomainsType = 67 // deprecated
	Tesla         GlobalEquivalentDomainsType = 68
	MorganStanley GlobalEquivalentDomainsType = 69
	TaxAct        GlobalEquivalentDomainsType = 70
	Wikimedia     GlobalEquivalentDomainsType = 71
	Airbnb        GlobalEquivalentDomainsType = 72
	Eventbrite    GlobalEquivalentDomainsType = 73
	StackExchange GlobalEquivalentDomainsType = 74
)

// GlobalDomains is the list of the global equivalent domains.
// https://github.com/bitwarden/server/blob/master/src/Core/Utilities/StaticStore.cs
var GlobalDomains = map[GlobalEquivalentDomainsType][]string{
	Ameritrade:    {"ameritrade.com", "tdameritrade.com"},
	BoA:           {"bankofamerica.com", "bofa.com", "mbna.com", "usecfo.com"},
	Sprint:        {"sprint.com", "sprintpcs.com", "nextel.com"},
	Google:        {"youtube.com", "google.com", "gmail.com"},
	Apple:         {"apple.com", "icloud.com"},
	WellsFargo:    {"wellsfargo.com", "wf.com"},
	Merrill:       {"mymerrill.com", "ml.com", "merrilledge.com"},
	Citi:          {"accountonline.com", "citi.com", "citibank.com", "citicards.com", "citibankonline.com"},
	Cnet:          {"cnet.com", "cnettv.com", "com.com", "download.com", "news.com", "search.com", "upload.com"},
	Gap:           {"bananarepublic.com", "gap.com", "oldnavy.com", "piperlime.com"},
	Microsoft:     {"bing.com", "hotmail.com", "live.com", "microsoft.com", "msn.com", "passport.net", "windows.com", "microsoftonline.com", "office365.com", "microsoftstore.com", "xbox.com"},
	United:        {"ua2go.com", "ual.com", "united.com", "unitedwifi.com"},
	Yahoo:         {"overture.com", "yahoo.com"},
	Zonelabs:      {"zonealarm.com", "zonelabs.com"},
	PayPal:        {"paypal.com", "paypal-search.com"},
	Avon:          {"avon.com", "youravon.com"},
	Diapers:       {"diapers.com", "soap.com", "wag.com", "yoyo.com", "beautybar.com", "casa.com", "afterschool.com", "vine.com", "bookworm.com", "look.com", "vinemarket.com"},
	Contacts:      {"1800contacts.com", "800contacts.com"},
	Amazon:        {"amazon.com", "amazon.co.uk", "amazon.ca", "amazon.de", "amazon.fr", "amazon.es", "amazon.it", "amazon.com.au", "amazon.co.nz", "amazon.in"},
	Cox:           {"cox.com", "cox.net", "coxbusiness.com"},
	Norton:        {"mynortonaccount.com", "norton.com"},
	Verizon:       {"verizon.com", "verizon.net"},
	Buy:           {"rakuten.com", "buy.com"},
	Sirius:        {"siriusxm.com", "sirius.com"},
	Ea:            {"ea.com", "origin.com", "play4free.com", "tiberiumalliance.com"},
	Basecamp:      {"37signals.com", "basecamp.com", "basecamphq.com", "highrisehq.com"},
	Steam:         {"steampowered.com", "steamcommunity.com", "steamgames.com"},
	Chart:         {"chart.io", "chartio.com"},
	Gotomeeting:   {"gotomeeting.com", "citrixonline.com"},
	Gogo:          {"gogoair.com", "gogoinflight.com"},
	Oracle:        {"mysql.com", "oracle.com"},
	Discover:      {"discover.com", "discovercard.com"},
	Dcu:           {"dcu.org", "dcu-online.org"},
	Healthcare:    {"healthcare.gov", "cms.gov"},
	Pepco:         {"pepco.com", "pepcoholdings.com"},
	Century21:     {"century21.com", "21online.com"},
	Comcast:       {"comcast.com", "comcast.net", "xfinity.com"},
	Cricket:       {"cricketwireless.com", "aiowireless.com"},
	Mtb:           {"mandtbank.com", "mtb.com"},
	Dropbox:       {"dropbox.com", "getdropbox.com"},
	Snapfish:      {"snapfish.com", "snapfish.ca"},
	Alibaba:       {"alibaba.com", "aliexpress.com", "aliyun.com", "net.cn", "www.net.cn"},
	Playstation:   {"playstation.com", "sonyentertainmentnetwork.com"},
	Mercado:       {"mercadolivre.com", "mercadolivre.com.br", "mercadolibre.com", "mercadolibre.com.ar", "mercadolibre.com.mx"},
	Zendesk:       {"zendesk.com", "zopim.com"},
	Autodesk:      {"autodesk.com", "tinkercad.com"},
	RailNation:    {"railnation.ru", "railnation.de", "rail-nation.com", "railnation.gr", "railnation.us", "trucknation.de", "traviangames.com"},
	Wpcu:          {"wpcu.coop", "wpcuonline.com"},
	Mathletics:    {"mathletics.com", "mathletics.com.au", "mathletics.co.uk"},
	Discountbank:  {"discountbank.co.il", "telebank.co.il"},
	Mi:            {"mi.com", "xiaomi.com"},
	Postepay:      {"postepay.it", "poste.it"},
	Facebook:      {"facebook.com", "messenger.com"},
	Skysports:     {"skysports.com", "skybet.com", "skyvegas.com"},
	Disney:        {"disneymoviesanywhere.com", "go.com", "disney.com", "dadt.com"},
	Pokemon:       {"pokemon-gl.com", "pokemon.com"},
	Uv:            {"myuv.com", "uvvu.com"},
	Mdsol:         {"mdsol.com", "imedidata.com"},
	Yahavo:        {"bank-yahav.co.il", "bankhapoalim.co.il"},
	Sears:         {"sears.com", "shld.net"},
	Xiami:         {"xiami.com", "alipay.com"},
	Belkin:        {"belkin.com", "seedonk.com"},
	Turbotax:      {"turbotax.com", "intuit.com"},
	Shopify:       {"shopify.com", "myshopify.com"},
	Ebay:          {"ebay.com", "ebay.de", "ebay.ca", "ebay.in", "ebay.co.uk", "ebay.com.au"},
	Techdata:      {"techdata.com", "techdata.ch"},
	Schwab:        {"schwab.com", "schwabplan.com"},
	Tesla:         {"tesla.com", "teslamotors.com"},
	MorganStanley: {"morganstanley.com", "morganstanleyclientserv.com", "stockplanconnect.com", "ms.com"},
	TaxAct:        {"taxact.com", "taxactonline.com"},
	Wikimedia:     {"mediawiki.org", "wikibooks.org", "wikidata.org", "wikimedia.org", "wikinews.org", "wikipedia.org", "wikiquote.org", "wikisource.org", "wikiversity.org", "wikivoyage.org", "wiktionary.org"},
	Airbnb:        {"airbnb.at", "airbnb.be", "airbnb.ca", "airbnb.ch", "airbnb.cl", "airbnb.co.cr", "airbnb.co.id", "airbnb.co.in", "airbnb.co.kr", "airbnb.co.nz", "airbnb.co.uk", "airbnb.co.ve", "airbnb.com", "airbnb.com.ar", "airbnb.com.au", "airbnb.com.bo", "airbnb.com.br", "airbnb.com.bz", "airbnb.com.co", "airbnb.com.ec", "airbnb.com.gt", "airbnb.com.hk", "airbnb.com.hn", "airbnb.com.mt", "airbnb.com.my", "airbnb.com.ni", "airbnb.com.pa", "airbnb.com.pe", "airbnb.com.py", "airbnb.com.sg", "airbnb.com.sv", "airbnb.com.tr", "airbnb.com.tw", "airbnb.cz", "airbnb.de", "airbnb.dk", "airbnb.es", "airbnb.fi", "airbnb.fr", "airbnb.gr", "airbnb.gy", "airbnb.hu", "airbnb.ie", "airbnb.is", "airbnb.it", "airbnb.jp", "airbnb.mx", "airbnb.nl", "airbnb.no", "airbnb.pl", "airbnb.pt", "airbnb.ru", "airbnb.se"},
	Eventbrite:    {"eventbrite.at", "eventbrite.be", "eventbrite.ca", "eventbrite.ch", "eventbrite.cl", "eventbrite.co.id", "eventbrite.co.in", "eventbrite.co.kr", "eventbrite.co.nz", "eventbrite.co.uk", "eventbrite.co.ve", "eventbrite.com", "eventbrite.com.au", "eventbrite.com.bo", "eventbrite.com.br", "eventbrite.com.co", "eventbrite.com.hk", "eventbrite.com.hn", "eventbrite.com.pe", "eventbrite.com.sg", "eventbrite.com.tr", "eventbrite.com.tw", "eventbrite.cz", "eventbrite.de", "eventbrite.dk", "eventbrite.fi", "eventbrite.fr", "eventbrite.gy", "eventbrite.hu", "eventbrite.ie", "eventbrite.is", "eventbrite.it", "eventbrite.jp", "eventbrite.mx", "eventbrite.nl", "eventbrite.no", "eventbrite.pl", "eventbrite.pt", "eventbrite.ru", "eventbrite.se"},
	StackExchange: {"stackexchange.com", "superuser.com", "stackoverflow.com", "serverfault.com", "mathoverflow.net", "askubuntu.com"},
}
