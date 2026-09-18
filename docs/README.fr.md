![VoHiveX, plateforme personnelle de gestion et de test de modems](images/vohivex-banner.png)

<p align="center">
  <a href="https://github.com/iniwex5/vohive">VoHive</a> ·
  <a href="https://go.dev/">Go</a> ·
  <a href="https://github.com/MetaCubeX/mihomo">Mihomo</a> ·
  <a href="https://github.com/vuejs/core">Vue 3</a> ·
  <a href="https://github.com/vitejs/vite">Vite</a> ·
  <a href="https://github.com/vuejs/pinia">Pinia</a> ·
  <a href="https://github.com/antdv-next/antdv-next">Antdv Next</a> ·
  <a href="https://github.com/Remix-Design/RemixIcon">Remix Icon</a>
</p>

# VoHiveX

[English](../README.md) | [العربية](README.ar.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | Français | [Русский](README.ru.md) | [Español](README.es.md) | [日本語](README.ja.md)

**Projet d'origine :** [VoHive](https://github.com/iniwex5/vohive) par [iniwex5](https://github.com/iniwex5)<br>
**Auteur de VoHiveX :** [NXN-MAX](https://github.com/NXN-MAX) · **Version :** 2.1.4

VoHiveX est une plateforme personnelle de gestion et de test de modems dérivée de VoHive. Elle ajoute la compatibilité avec les modules DJI 4G, l'envoi de SMS planifiés, la gestion des abonnements et des nœuds Mihomo, la reconnaissance des codes d'activation eSIM, l'archivage des SMS, les notifications et une interface Vue adaptative soutenue par une passerelle Go.

## Fonctionnalités principales

### Compatibilité avec les modules DJI 4G

- Prend en charge les modules DJI 4G de première génération utilisant l'identifiant USB `2ca3:4006`.
- Conserve l'identifiant USB DJI d'origine. Aucun reflashage, changement d'identifiant USB ou modification permanente des pilotes de l'hôte n'est nécessaire.
- Associe à l'exécution l'appareil aux pilotes Linux `option` et `qmi_wwan` existants, puis expose au conteneur les interfaces série AT et QMI.
- Les modules de deuxième génération sont pris en charge via un chemin QMI ou MBIM compatible fourni par leur micrologiciel et le noyau de l'hôte.

### SMS planifiés

- Envoi unique à une date et une heure précises, ou répétition par intervalles de jours, heures, minutes et secondes.
- Création, modification, suppression, démarrage et pause des tâches, avec affichage de la prochaine exécution et de l'historique.
- Toute tâche créée ou modifiée reste en pause jusqu'à son démarrage manuel. Les exécutions manquées ne sont pas envoyées en rafale.
- Les résultats d'exécution et de livraison peuvent être transmis aux canaux Telegram, Feishu, QQ, Bark, Email, Pushplus et Webhook activés.

### Gestion des abonnements et des nœuds

- Exécute Mihomo dans le conteneur VoHiveX et fournit un point SOCKS5 intégré fixe à `127.0.0.1:17890`.
- Prend en charge plusieurs abonnements HTTPS, les fichiers Clash YAML et les liens de partage de proxy courants.
- Fournit des listes d'abonnements repliables, la sélection des nœuds, les tests de latence, les règles VoWiFi par pays et les instances de proxy sortant local.
- Les images QR de proxy sont décodées dans le navigateur puis supprimées immédiatement après la reconnaissance.

### Reconnaissance des codes d'activation eSIM

- Reconnaît les images QR, les images du presse-papiers, les fichiers JPG/JPEG/PNG/WebP importés et les liens `LPA:1` saisis directement.
- Analyse l'adresse SM-DP+, le Matching ID et le code de confirmation facultatif, puis renseigne le formulaire de téléchargement.
- La reconnaissance ne fait que remplir le formulaire. Aucun Profile n'est écrit avant vérification et clic sur **Démarrer le téléchargement**.
- Affiche les informations eUICC et les Profile installés, avec notes, changement et suppression lorsque le module et la carte le permettent.

### Gestion des appareils et des messages

- Découverte des appareils, état radio, terminaux AT et USSD, informations SIM/eSIM, politiques de carte, état VoWiFi et journaux en direct.
- Centre SMS à trois colonnes avec recherche de conversations, état de lecture, état de livraison, actions multiples et import/export CSV/TXT/HTML/XML.
- Swagger UI local sur `/api/docs`, contrôle d'état sur `/healthz` et métriques compatibles Prometheus sur `/metrics`.

## Architectures et chemins de modem pris en charge

| Composant | Cibles prises en charge |
| --- | --- |
| Environnement Docker complet | Linux `amd64`, `arm64`/`aarch64` et `armv7` |
| Passerelle Go autonome | Linux `amd64`, `arm64`/`aarch64`, `armv7` et `386` |
| Transport du modem | Série AT avec réseau QMI ou MBIM compatible Qualcomm |
| Interfaces du noyau hôte | `option`, `qmi_wwan`, série USB, périphérique de contrôle QMI ou chemin MBIM compatible |
| DJI première génération | USB ID `2ca3:4006`, sans modifier l'identifiant USB |
| DJI deuxième génération | Dispositions QMI/MBIM compatibles ; fonctions selon le micrologiciel et la composition USB |

Le conteneur complet n'est publié que pour les architectures disposant d'un cœur de modem intégré compatible. Le téléchargement `386` contient uniquement la passerelle Go et doit être relié à un cœur de modem compatible fourni séparément.

## Installation avec Docker

Les images sont publiées sur :

- `maxnxxn/vohivex:2.1.4`
- `ghcr.io/nxn-max/vohivex:2.1.4`

Les deux dépôts proposent les balises multi-architectures `2.1.4`, `v2.1.4` et `latest`. Docker sélectionne automatiquement l'image adaptée à l'hôte.

Valeurs par défaut :

- Port Web : `7575`
- Nom d'utilisateur : `admin`
- Mot de passe : `admin`

```sh
cp .env.example .env
docker compose pull
docker compose up -d
```

Ouvrez `http://<adresse-hote>:7575`. Les répertoires `config`, `data`, `logs` et `driver-state` existants sont conservés pendant les mises à jour. Modifiez le mot de passe par défaut après la première connexion.

Le conteneur détecte le noyau en cours d'exécution sur l'hôte et réutilise ses pilotes de modem. Il n'installe aucun paquet du noyau et ne remplace pas le noyau de l'hôte.

## Captures d'écran

Toutes les captures ci-dessous sont générées par l'API de démonstration locale. Les identifiants d'appareils, adresses IP, numéros de téléphone, messages, abonnements, nœuds, données eSIM et tâches sont fictifs.

| Tableau de bord | Abonnements et nœuds |
| --- | --- |
| ![Tableau de bord VoHiveX avec des données de modem fictives](images/dashboard.png) | ![Abonnements proxy VoHiveX avec des nœuds fictifs](images/proxy.png) |

| Reconnaissance eSIM par QR ou lien | Centre SMS |
| --- | --- |
| ![Reconnaissance d'activation eSIM VoHiveX avec des données d'exemple](images/esim.png) | ![Centre SMS VoHiveX avec des conversations fictives](images/sms.png) |

### Tâches planifiées

![Tâches planifiées VoHiveX avec des destinataires et contenus fictifs](images/tasks.png)

## Versions et documentation

- [Versions GitHub](https://github.com/NXN-MAX/VoHiveX/releases)
- [Docker Hub](https://hub.docker.com/r/maxnxxn/vohivex)
- [Pilotes et déploiement](../app/README.md)
- [SMS planifiés](../app/scheduler/README.md)
- [Mihomo et abonnements](../app/proxy/README.md)
- [Mentions relatives aux composants tiers](../app/proxy/THIRD-PARTY.md)

## Utilisation responsable et licence

> [!CAUTION]
> **Toute utilisation commerciale est strictement interdite. VoHiveX est réservé à la recherche, à l'apprentissage et aux tests personnels sur des appareils et des numéros de téléphone que vous contrôlez légalement.**

N'utilisez pas VoHiveX pour collecter des codes de vérification, louer des numéros, envoyer des messages non sollicités ou en masse, commettre une fraude, fournir des services proxy illicites, ni pour toute activité contraire au droit local ou aux conditions d'un opérateur.

Le projet d'origine et les composants tiers conservent leurs licences respectives. Les ajouts VoHiveX utilisent la [licence personnelle non commerciale](../LICENSE), qui n'est pas une licence open source approuvée par l'OSI.
