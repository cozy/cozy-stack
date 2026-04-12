---
phase: 03-copy-compliance-and-documentation
type: manual-validation
date: 2026-04-12
status: passed
requirements: [TEST-05]
---

# Validation manuelle — OnlyOffice Documents Android contre WebDAV Cozy

## Résultat

**✓ PASSED** — Toutes les opérations testées depuis le client mobile OnlyOffice passent contre la route WebDAV Cozy.

## Contexte

Le scope reduction de Phase 3 a officiellement déféré la validation manuelle OnlyOffice mobile à v1.1 (bug client upstream `LoginComponent null` / "App token login name does not match" dans OO Documents v9.2.0+). Cette validation manuelle a été menée de manière opportuniste avec une version **antérieure à la régression** pour confirmer expérimentalement que le serveur Cozy WebDAV est correct — l'erreur est bien purement côté app.

Cette entrée **ne remet pas en cause** le scope reduction documenté dans `REQUIREMENTS.md` (les utilisateurs finaux sur Play Store/App Store sont sur v9.3.1 qui a la régression), mais apporte une **preuve expérimentale supplémentaire** que l'implémentation serveur est fonctionnelle pour un vrai client mobile OO.

## Setup

| Élément | Valeur |
|---------|--------|
| Client | OnlyOffice Documents Android **v9.1.0** (build 9.1.0-663) |
| Source APK | `github.com/ONLYOFFICE/documents-app-android/releases/tag/v9.1.0-663` — asset `onlyoffice-manager-9.1.0-663.apk` |
| Installation | `adb install -r` (side-load) après désinstallation de la 9.3.1 Play Store |
| Device | Google Pixel 10, Android 16, arm64-v8a |
| Serveur | cozy-stack branche `feat/webdav` (commit 32651eed3+) |
| Instance Cozy | `192-168-1-189.nip.io:8080` (alias `127.0.0.1:8080`) |
| Route WebDAV testée | `/dav/files/` |
| Auth | Basic, username vide, password = CLI token scope `io.cozy.files` |
| Connectivité | Tunnel USB `adb reverse tcp:8080 tcp:8080` (firewall/VPN LAN contournés) |
| OnlyOffice DocumentServer | Container `oo-dev` en mode `host` sur port 80, JWT désactivé |

## Scope testé depuis le client mobile

Toutes les opérations listées ci-dessous ont été validées sans erreur côté client :

- Authentification initiale (PROPFIND racine)
- Navigation dans l'arborescence (listing des dossiers enfants)
- Lecture de fichiers (GET / download)
- Création de fichiers (PUT)
- Création de dossiers (MKCOL)
- Renommage / déplacement (MOVE)
- Copie (COPY)
- Suppression (DELETE)

Confirmé par l'utilisateur : *"c'est tout bon, les tests sont tous ok !"*.

## Ce que cette validation ajoute aux garanties existantes

Avant cette validation manuelle, la conformité WebDAV reposait sur trois couches automatisées :

1. **Tests Go unitaires** (`web/webdav/*_test.go`) — 50+ tests, invariants protocolaires
2. **Tests E2E gowebdav** (`gowebdav_integration_test.go`) — 6 success-criteria sub-tests via un vrai client Go
3. **Litmus** (`scripts/webdav-litmus.sh`) — suite de conformité RFC 4918 externe, 63 tests sur 4 suites (basic 16/16, copymove 13/13, props 30/30, http 4/4) contre les deux routes

Cette validation ajoute une **quatrième couche, humaine et in-vivo** : un vrai client mobile grand public, avec son propre stack HTTP / auth / XML parser / retry logic, qui se connecte et exécute les opérations usuelles. Cela confirme que l'interopérabilité n'est pas un artefact des tests automatisés.

## Découvertes annexes

### 1. Écart de nommage du bug upstream

La documentation interne de Phase 3 (CONTEXT, PLAN, REQUIREMENTS) nommait le bug client **"LoginComponent null"**. L'investigation sur les forums ONLYOFFICE (thread *iOS app 9.2 breaks WebDAV/Nextcloud support!*) a révélé que le vrai message d'erreur upstream est **"App token login name does not match"**. Les deux désignent la même régression introduite en 9.2.0, mais pour la traçabilité future il vaut mieux se référer au message upstream.

Aucune issue GitHub ne tracke ce bug sur les repos `ONLYOFFICE/documents-app-android` ou `ONLYOFFICE/documents-app-ios` (avril 2026). Le suivi se fait uniquement sur le forum communautaire. Pas de version 9.3.2 publiée, pas de beta publique annoncée.

### 2. Connectivité LAN vs tunnel USB

Le test initial via `http://192-168-1-189.nip.io:8080/` (résolution via nip.io → IP LAN) n'a pas fonctionné : le téléphone ne pouvait pas joindre le PC malgré une présence confirmée sur le même WiFi. Cause probable : VPN actif sur le téléphone (interface `rmnet1` active en parallèle du `wlan0`, route par défaut détournée via `10.2.0.2`) ou firewall PC. Le contournement via `adb reverse tcp:8080 tcp:8080` bypass complètement le réseau IP et crée un tunnel USB direct.

Cette leçon peut intéresser d'autres tests manuels depuis mobile : **`adb reverse` est plus fiable que le LAN pour le dev**, et ne nécessite aucune configuration réseau côté téléphone.

### 3. Alias de domaine Cozy

L'accès via tunnel USB nécessite que le Host HTTP reçu par le serveur (`127.0.0.1:8080`) corresponde à un domaine connu. Plutôt que de créer une seconde instance, l'alias a été ajouté via :

```
cozy-stack instances modify 192-168-1-189.nip.io:8080 --domain-aliases "127.0.0.1:8080"
```

Le même token CLI reste valide pour les deux domaines. Pattern utile pour les setups de dev.

## Liens

- [ONLYOFFICE/documents-app-android v9.1.0-663 release](https://github.com/ONLYOFFICE/documents-app-android/releases/tag/v9.1.0-663)
- [Thread forum ONLYOFFICE — bug v9.2+](https://community.onlyoffice.com/t/ios-app-9-2-breaks-webdav-nextcloud-support/17415)
- `.planning/REQUIREMENTS.md` → section *Scope reductions (Phase 3)* — décision documentée du deferral
- `docs/webdav.md` → section *Compliance testing* — procédure litmus
