# Format de fichier

Les applications cozy peuvent utiliser des fichiers pour stocker du contenu
binaire, comme des photos ou des PDF. Les métadonnées sont conservées dans
CouchDB, mais les binaires peuvent accéder au système de fichier ou à une
instance Swift.

## Les formats standards utilisés pour :

### Les fichiers/dossiers

* les attributs étendus permettent à l’utilisateur d’un systeme de fichier
  d’associer des métadonnées. `https://godoc.org/github.com/ivaxer/go-xattr`
  librairie go supportant les attributs étendus

* Extensible Metadata Platform ou XMP est un format de métadonnées basé sur XML.
  XMP permet d'enregistrer sous forme d'un document XML des informations
  relatives à un fichier XMP définit différentes méthodes pour stocker ce
  document XML au sein même de fichiers JPEG, GIF, HTML….

* stocker les fichiers json avec les métadonnées dans un autre répertoire et les
  fichiers binaires à part pour pouvoir récupérer tous les fichiers lors de
  l’export et remettre tout ca correctement lors de l’import avec les json.

### Les albums (?)

* répertoires avec le nom de l’album contenant les photos associées

### Les contacts

Plusieurs formats sont utilisés et ceci peut provoquer des incompatibilités
entre differents clients. Google propose par exemple au client les deux types de
formats (vcard et csv) (Il prend en charge l'importation de fichiers CSV à
partir d'Outlook, d'Outlook Express, de Yahoo! Mail, de Hotmail, d'Eudora et de
certaines autres applications. Il prend également en charge l'importation de
fichiers vCard à partir d'applications telles que le Carnet d'adresses Apple.)
Les contacts iCloud sont importés et exportés au format vCard

* vcard est un format standard ouvert d'échange de données personnelles (fichier
  d’extension .vcf)

Exemple

```
BEGIN:VCARD
VERSION:2.1
FN:Jean Dupont
N:Dupont;Jean
ADR;WORK;PREF;QUOTED-PRINTABLE:;Bruxelles 1200=Belgique;6A Rue Th. Decuyper
LABEL;QUOTED-PRINTABLE;WORK;PREF:Rue Th. Decuyper 6A=Bruxelles 1200=Belgique
TEL;CELL:+1234 56789
EMAIL;INTERNET:jean.dupont@example.com
UID:
END:VCARD
```

* csv format utilisé pour les contacts également

Exemple

```
"Prenom","Nom","Email","Age"
"Jean", "Petit", "jean@monsite.fr", "34"
"Anne", "Le Gall", "anne@exemple.net", "21"
"Pierre", "Diawara", "pierre@sonsite.com", "44"
```

### Les calendriers

* iCalendar est un format de données défini pour les échanges de données de
  calendrier (fichier d’extension .ical, ou .ics comme owncloud)

Exemple

```
BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//hacksw/handcal//NONSGML v1.0//EN
BEGIN:VEVENT
DTSTART:19970714T170000Z
DTEND:19970715T035959Z
SUMMARY:Fête à la Bastille
END:VEVENT
END:VCALENDAR
```

## Les formats utilisés dans Cozy pour :

### Les fichiers/dossiers

* Un fichier est un contenu binaire avec certaines métadonnées. Un fichier Json
  dans CouchDB contient un champ `id`, un champ `rev` et des `métadonnées`. Ce
  fichier est relié au fichier binaire qui se trouve dans le systeme de fichier.
  Un fichier a un champ parent et lorsqu'il est deplacé dans l'arboresence on ne
  modifie que le fichier Json sans toucher au contenu dans le systeme de
  fichier. Dans CouchDB, les fichers sont indexés et ont une structure
  arborescente.

### Les albums

* L'album est un document dans CouchDB qui va lister les photos qu'il contient.
