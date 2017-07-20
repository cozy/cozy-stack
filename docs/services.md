[Table of contents](README.md#table-of-contents)

# Services, Notifications et Konnectors

## Notifications

Inspiration : https://developer.android.com/guide/topics/ui/notifiers/notifications.html

Notifications are couchdb document with doctype `io.cozy.notifications`.

Schema will be defined in cozy-doctypes, along the lines of
```
notificationRef // useful to update a notification (X mails unread)
notificationType: // useful to "hide notif like this"
source: applicationID
title: text
content?: text
icon?: image
actions?: [{text, intents}]
```

All applications/service/konnector can create notifications.

- notifications will appears in the cozy-bar
- Cozy mobile app(s) may display notifications on mobile.
- Some notifications may also be transmitted by email, eventually as summaries over period of time.
- The settings/notifications app will have "notifications center" tab, allowing to silence notifications and pick which should be sent to mobile / mail

**WIP should we have a permission for notification creation?**
**WIP should notification have a type defined by the app, allowing finer control in the notification center?**


## Rappel de l'état actuel des brique technique

**Worker** (En dur dans le code) Fonction en go dans la stack

**Job** execution d'un worker avec des paramètres donnés

**Trigger** règle pour lancer un job à intervalle ou en réaction à un changement de la bdd avec des paramètres donnés.

**WebApp:** (installable) Tarball de code executable dans le Browser + Permissions associées

**Konnector:** (installable) Tarball de code executable dans Node-JS + Permissions associées

**KonnectorWorker** Un Worker qui execute un Konnector en lui transmettant les paramètre.


## Equivalence ressenti utilisateur <-> brique technique

"J'ai configuré le connecteur XXXX" =
- Un account ID: "YYYY" et type: "XXXX" avec les paramètre de connection
- Un Trigger configuré pour appeler le KonnectorWorker à intervalle régulier, avec en paramètre {accountID: "YYYY", konnector:"XXXX"}


**La stack n'a aucune notion du fait qu'un konnector récupère des données.**

## Séparation apps / konnector, une aide à la privacy&sécurité

La séparation technique app et konnector a pour vocation de renforcer la sécurité et privacy:

  - le konnector bancaire a besoin de lire le login et d'écrire des opérations,
  il a accès à internet, c'est pas grave, il n'a pas accès aux données privées.

  - la web-app bancaire peut demander l'accès aux events & contacts pour faire des matchup, c'est pas grave, elle n'a pas accès à internet.


# Use case to be considered for services & konnectors

**UC1: Messages Editoriaux**
Partenaire veut proposer à l'utilisateur des messages utiles, si l'utilisateur est dans une catégorie donnée.
Partenaire n'a pas envie de savoir que tel cozy est dans tel catégorie
Partenaire publie TOUS LES MESSAGES sur une API.

**UC2: Alerte sans accéder à l'application**
Une application veut lancer un bout de code à intervalle régulier et créer des notifications  

**UC3: Notifications bancaire**
Selon des règles trop complexe pour être exprimé autrement que par du code (réduction des opérations bancaire < seuil défini dans les settings de l'application bancaire), je veux créer ou non une notification.

**UC4: Un konnector peut créer une notifications en cas d'échec**

**UC5: Je peux recevoir un rapport mensuel de mes factures détaillées**

# Application aux use-cases

**UC1**
L'application crée un konnector et un service
- Le konnector, avec le consentement de l'utilisateur, accède au site du partenaire et rapatrie toutes les offres
- Le service, trigger sur la modification des offres, crée des notifications

**UC2**
L'application crée un service avec un trigger cron

**UC3**
L'application crée un service avec un trigger sur les opérations bancaire

**UC4**
Le konnecteur crée une notification

**UC5**
Tous les konnecteur crée des notifications en cas de succès,
Le notification center est configuré par défaut pour summary de notif type "konnector_success"


# Solution proposé

1. Créer la notion de service

Un service fonctionne comme un connector, c'est un bout de code à lancer dans node-js. Un service est associé à un trigger.

Un service n'a pas accès à internet
Un service utilise un bout de code fournit par l'app (comme un ServiceWorker)
Ce bout de code a accès au cozy-client-js dans la variable globale cozy.
Toute autre librairie devra être packagée.

----------------------

2b. Permettre à une application d'inclure un konnector

Le konnector inclut a ses propres permissions.

L'utilisateur doit accepter la création du konnecteur
```
L'application XXX veut lancer un collecteur avec les permissions:
  - Lecture: type de contrat XXX  
  - Accès à internet
  - Creation: Offre commerciale XXX
```





UC1: combinaison konnector récupère les données et service crée la notification si besoin

UC2:


# Etapes

- Renommer le worker konnector en jsworker
- Rajouter une permission "WebAccess", tous les konnector existant doivent la demander (inclure le website dans cette permission ?)
- Modifier le jsworker pour qu'il puisse aussi executer un fichier inclus dans la tarball d'une app.
