<p align="center">
  <img src="assets/logo.png" alt="Claude Code Go Logo" width="200">
</p>

<h1 align="center">Claude Code Go</h1>

<p align="center">
  <strong>🤖 Réimplémentation en Go de Claude Code — Assistant de programmation IA dans votre terminal</strong>
</p>

<p align="center">
  <a href="https://golang.org/dl/"><img src="https://img.shields.io/badge/go-1.21+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Version Go"></a>
  <a href="https://goreportcard.com/report/github.com/tunsuy/claude-code-go"><img src="https://goreportcard.com/badge/github.com/tunsuy/claude-code-go?style=flat-square" alt="Go Report Card"></a>
  <a href="https://codecov.io/gh/tunsuy/claude-code-go"><img src="https://codecov.io/gh/tunsuy/claude-code-go/branch/main/graph/badge.svg?style=flat-square" alt="Couverture"></a>
  <a href="https://pkg.go.dev/github.com/tunsuy/claude-code-go"><img src="https://pkg.go.dev/badge/github.com/tunsuy/claude-code-go.svg" alt="Go Reference"></a>
  <a href="https://github.com/tunsuy/claude-code-go/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/tunsuy/claude-code-go/ci.yml?branch=main&style=flat-square&logo=github&label=CI" alt="CI"></a>
  <a href="https://github.com/tunsuy/claude-code-go/releases"><img src="https://img.shields.io/github/v/release/tunsuy/claude-code-go?style=flat-square&logo=github" alt="Release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-green?style=flat-square" alt="Licence"></a>
  <a href="https://github.com/tunsuy/claude-code-go/pulls"><img src="https://img.shields.io/badge/PRs-welcome-brightgreen?style=flat-square" alt="PRs Welcome"></a>
</p>

<p align="center">
  <a href="README.md">English</a> •
  <a href="README.zh-CN.md">简体中文</a> •
  <a href="README.ja.md">日本語</a> •
  <a href="README.ko.md">한국어</a> •
  <a href="README.es.md">Español</a> •
  <a href="README.fr.md">Français</a>
</p>

---

<p align="center">
  <img src="assets/image.png" alt="Claude Code Go - Session Interactive" width="800">
  <br>
  <em>TUI interactive avec lecture de fichiers et affichage de la réflexion en temps réel</em>
</p>

<p align="center">
  <img src="assets/image1.png" alt="Claude Code Go - Analyse de Projet" width="800">
  <br>
  <em>Analyse complète du projet avec détails de l'architecture</em>
</p>

---

## Qu'est-ce que c'est ?

Ce projet est une **réimplémentation complète en Go de [Claude Code](https://claude.ai/code)** — le CLI TypeScript officiel d'Anthropic — réécrit module par module en Go, couvrant toutes les fonctionnalités principales : TUI, utilisation d'outils, système de permissions, coordination multi-agents, protocole MCP, gestion des sessions, et plus encore.

> **🤖 Entièrement construit par des agents IA — zéro code de production écrit par des humains.** La construction initiale (avril 2026) a été produite par 9 agents IA Claude collaborant en 9 jours ; depuis août 2026, une session Claude principale unique maintient le projet en s'appuyant sur un registre des écarts de parité comportementale, avec un responsable humain qui assure la revue et les fusions. [Histoire complète ↓](#entièrement-construit-par-des-agents-ia)

## Fonctionnalités

- **TUI interactive** — Interface terminal complète construite avec [Bubble Tea](https://github.com/charmbracelet/bubbletea), avec thèmes sombre/clair
- **Utilisation d'outils agentiques** — 34 outils intégrés (fichier, shell, recherche, web, tâches, sous-agents, …), tous médiés par un pipeline de permissions à 9 couches
- **Sous-commandes CLI** — `claude mcp …`, `claude plugin …`, `claude auth …`, `claude doctor`, `claude update` ; les groupes `mcp` et `plugin` produisent une sortie identique octet par octet à celle du CLI TypeScript
- **Gestion des plugins** — Installation/activation/désactivation de plugins depuis des marketplaces locaux, avec validation complète des manifestes (`claude plugin validate`)
- **Coordination multi-agents** — Lance des sous-agents en arrière-plan pour des tâches parallèles
- **Support MCP** — Connecte des outils externes via le [Model Context Protocol](https://modelcontextprotocol.io) (transports stdio + HTTP), importables depuis Claude Desktop
- **Mémoire CLAUDE.md** — Charge automatiquement le contexte du projet depuis les fichiers `CLAUDE.md` tout au long de l'arborescence
- **Gestion des sessions** — Reprend les conversations précédentes ; compacte automatiquement les historiques longs
- **Mode Vim** — Raccourcis clavier Vim optionnels dans la zone de saisie
- **Authentification OAuth + clé API** — Connectez-vous avec OAuth Anthropic (`claude auth login`) ou fournissez une `ANTHROPIC_API_KEY`
- **Commandes slash intégrées** — `/compact`, `/commit`, `/review`, `/model`, `/theme`, et plus (voir ci-dessous)
- **Réponses en streaming** — Streaming de tokens en temps réel avec affichage des blocs de réflexion

## Architecture

Claude Code Go est organisé en six couches :

```
┌─────────────────────────────────────┐
│  CLI (cmd/claude)                   │  point d'entrée Cobra
├─────────────────────────────────────┤
│  TUI (internal/tui)                 │  interface Bubble Tea MVU
├─────────────────────────────────────┤
│  Tools (internal/tools)             │  outils fichier, shell, recherche, MCP
├─────────────────────────────────────┤
│  Core Engine (internal/engine)      │  streaming, dispatch d'outils, coordinateur
├─────────────────────────────────────┤
│  Services (internal/api, oauth,     │  API Anthropic, OAuth, client MCP
│            mcp, compact)            │
├─────────────────────────────────────┤
│  Infra (pkg/types, internal/config, │  types, configuration, état, hooks, plugins
│         state, session, hooks)      │
└─────────────────────────────────────┘
```

Voir [`docs/project/architecture.md`](docs/project/architecture.md) pour une description détaillée.

## Prérequis

- Go 1.21 ou ultérieur
- Une [clé API Anthropic](https://console.anthropic.com/) **ou** un compte Claude.ai (OAuth)

## Installation

### Depuis les sources

```bash
git clone https://github.com/tunsuy/claude-code-go.git
cd claude-code-go
make build
# Le binaire est placé dans ./bin/claude
```

Ajouter à votre `PATH` :

```bash
export PATH="$PATH:$(pwd)/bin"
```

### Avec `go install`

```bash
go install github.com/tunsuy/claude-code-go/cmd/claude@latest
```

## Démarrage rapide

```bash
# Configurez votre clé API (ou utilisez OAuth — voir Authentification ci-dessous)
export ANTHROPIC_API_KEY=sk-ant-...

# Démarrez une session interactive dans le répertoire courant
claude

# Posez une question unique et quittez
claude -p "Expliquez le point d'entrée principal de ce projet"

# Reprenez la session la plus récente
claude --resume
```

## Authentification

**Clé API (recommandé pour CI/scripts) :**

```bash
export ANTHROPIC_API_KEY=sk-ant-...
# ou stockez-la de façon persistante :
claude auth login --api-key sk-ant-...
```

**OAuth (recommandé pour une utilisation interactive) :**

```bash
claude auth login    # ouvre le flux OAuth dans votre navigateur
claude auth status   # vérifie avec quel compte vous êtes connecté
```

## Fournisseurs d'API

Claude Code Go prend en charge plusieurs fournisseurs d'API, vous permettant d'utiliser non seulement l'API d'Anthropic, mais aussi des APIs compatibles OpenAI.

### Fournisseurs pris en charge

| Fournisseur | Description | Variables d'environnement |
|-------------|-------------|--------------------------|
| `direct` (par défaut) | API directe d'Anthropic | `ANTHROPIC_API_KEY`, `ANTHROPIC_BASE_URL` |
| `openai` | OpenAI et APIs compatibles | `OPENAI_API_KEY`, `OPENAI_BASE_URL` |
| `bedrock` | AWS Bedrock | Identifiants AWS via environnement |
| `vertex` | Google Cloud Vertex AI | Identifiants GCP via environnement |

### Utilisation d'APIs compatibles OpenAI

Pour utiliser OpenAI, DeepSeek, Qwen, Moonshot ou toute API compatible OpenAI :

**Méthode 1 : Variables d'environnement**

```bash
# Définir le fournisseur à openai
export CLAUDE_PROVIDER=openai

# Définir votre clé API
export OPENAI_API_KEY=sk-xxx

# Optionnel : définir une URL de base personnalisée (pour les services compatibles OpenAI)
export OPENAI_BASE_URL=https://api.deepseek.com  # DeepSeek
# export OPENAI_BASE_URL=https://api.moonshot.cn/v1  # Moonshot
# export OPENAI_BASE_URL=http://localhost:11434/v1  # Ollama

# Définir le modèle
export OPENAI_MODEL=deepseek-chat

# Lancer Claude Code
claude
```

**Méthode 2 : Fichier de configuration**

Créez ou éditez `~/.config/claude-code/settings.json` :

```json
{
  "provider": "openai",
  "apiKey": "sk-xxx",
  "baseUrl": "https://api.openai.com",
  "model": "gpt-4-turbo",
  "openaiOrganization": "org-xxx",
  "openaiProject": "proj-xxx"
}
```

### Notes spécifiques par fournisseur

**OpenAI :**
- Prend en charge tous les modèles GPT-4 et GPT-3.5
- Support complet des outils/appels de fonctions
- Réponses en streaming

**DeepSeek :**
```bash
export CLAUDE_PROVIDER=openai
export OPENAI_API_KEY=sk-xxx
export OPENAI_BASE_URL=https://api.deepseek.com
export OPENAI_MODEL=deepseek-chat
```

**Ollama (local) :**
```bash
export CLAUDE_PROVIDER=openai
export OPENAI_BASE_URL=http://localhost:11434/v1
export OPENAI_MODEL=llama3
```

**Azure OpenAI :**
```bash
export CLAUDE_PROVIDER=openai
export OPENAI_API_KEY=your-azure-key
export OPENAI_BASE_URL=https://your-resource.openai.azure.com
export OPENAI_MODEL=your-deployment-name
```

**OrcaRouter (passerelle d'IA) :**
[OrcaRouter](https://www.orcarouter.ai/ref/ref_49eee7ba2e9927450075) est une passerelle d'IA qui route chaque requête vers le modèle le plus adapté parmi plus de 200 (de pointe et open source), aux prix des fournisseurs, avec basculement et journaux de requêtes. Compatible OpenAI, il fonctionne directement via le fournisseur `openai` :

```bash
export CLAUDE_PROVIDER=openai
export OPENAI_API_KEY=<your-orcarouter-key>
export OPENAI_BASE_URL=https://api.orcarouter.ai/v1
export OPENAI_MODEL=orcarouter/auto   # laisse OrcaRouter choisir le modèle pour chaque requête
```

*(Le lien ci-dessus est un lien de parrainage.)*

## Utilisation

### Mode interactif

```
claude [flags]
```

| Flag | Description |
|------|-------------|
| `--resume` | Reprendre la session la plus récente |
| `--session <id>` | Reprendre une session spécifique par ID |
| `--model <name>` | Remplacer le modèle Claude par défaut |
| `--dark` / `--light` | Forcer le thème sombre ou clair |
| `--vim` | Activer les raccourcis clavier Vim |
| `-p, --print <prompt>` | Non interactif : exécuter un seul prompt et quitter |

### Sous-commandes CLI

Commandes de gestion non interactives. Les groupes `mcp` et `plugin` sont compatibles octet par octet en sortie avec le CLI TypeScript :

| Commande | Description |
|----------|-------------|
| `claude mcp add <nom> -- <commande> [args…]` | Enregistrer un serveur MCP stdio (`--transport http <nom> <url>` pour un serveur distant, `-e KEY=VAL` pour les variables d'environnement) |
| `claude mcp list` / `claude mcp get <nom>` | Lister les serveurs configurés / vérifier l'état de l'un d'entre eux |
| `claude mcp add-json <nom> <json>` | Enregistrer un serveur à partir d'un JSON brut |
| `claude mcp add-from-claude-desktop` | Importer les serveurs depuis la configuration de Claude Desktop |
| `claude mcp remove <nom>` / `reset-project-choices` | Supprimer un serveur / réinitialiser les choix propres au projet |
| `claude plugin install <plugin>` | Installer un plugin depuis un marketplace configuré |
| `claude plugin list` / `enable` / `disable` / `uninstall` / `update` | Gérer les plugins installés |
| `claude plugin validate <chemin>` | Valider un manifeste de plugin ou de marketplace (`--strict`, `--json`) |
| `claude plugin marketplace add/list/remove/update` | Gérer les marketplaces de plugins (chemins locaux) |
| `claude auth login` / `logout` / `status` | Authentification OAuth ou par clé API |
| `claude doctor` | Vérification de l'état de l'environnement |

### Commandes slash

Tapez `/` dans la zone de saisie pour voir toutes les commandes disponibles :

| Commande | Description |
|----------|-------------|
| `/help` | Afficher toutes les commandes |
| `/clear` | Effacer l'historique de conversation |
| `/compact` | Résumer l'historique pour réduire l'utilisation du contexte |
| `/exit` | Quitter Claude Code |
| `/model` | Afficher ou définir le modèle actif |
| `/theme` | Changer de thème de couleurs |
| `/vim` | Basculer les raccourcis clavier Vim |
| `/effort` | Définir le niveau d'effort (bas / moyen / élevé) |
| `/status` | Afficher le statut de la session |
| `/cost` | Afficher l'utilisation des tokens pour cette session |
| `/session` | Afficher l'ID de la session en cours |
| `/memory` | Afficher les fichiers mémoire CLAUDE.md chargés |
| `/dream` | Déclencher manuellement la consolidation de la mémoire |
| `/review` | Examiner le diff git courant avec Claude |
| `/commit` | Demander à Claude de rédiger et de créer un commit git |
| `/diff` | Afficher le diff git courant |
| `/init` | Générer un CLAUDE.md pour ce projet |

`/config`, `/mcp`, `/resume` et `/terminal-setup` sont enregistrées mais pas encore implémentées — en attendant, utilisez les sous-commandes CLI ci-dessus pour la gestion des serveurs MCP.

## Entièrement construit par des agents IA

> **Aucun humain n'a écrit une seule ligne de code de production dans ce dépôt.**

L'ensemble du projet — conception de l'architecture, documents de conception détaillés, implémentation parallèle, revue de code, QA et tests d'intégration — a été produit par **9 agents IA Claude** collaborant dans un workflow multi-agents structuré :

```
Agent PM          →  plan de projet, jalons, planification des tâches
Agent Tech Lead   →  conception d'architecture, revue de documents de conception, revue de code
Agent-Infra       →  couche d'infrastructure (types, configuration, état, session)
Agent-Services    →  couche de services (client API, OAuth, MCP, compaction)
Agent-Core        →  moteur central (boucle LLM, dispatch d'outils, coordinateur)
Agent-Tools       →  couche d'outils (fichier, shell, recherche, web — 18 outils au moment de la construction)
Agent-TUI         →  couche UI (Bubble Tea MVU, thèmes, mode Vim)
Agent-CLI         →  couche d'entrée (Cobra CLI, DI, phases de bootstrap)
Agent QA          →  stratégie de test, acceptation par couche, tests d'intégration
```

Chaque agent travaillait sur une branche Git Worktree isolée, en parallèle, en collaborant via la base de code partagée, les documents de conception et les rapports QA. Résultat : ~**7 000 lignes de code de production + une suite de tests complète en 9 jours**, avec `go test -race ./...` qui passe.

C'est une démonstration concrète qu'une application Go multi-couches non triviale peut être entièrement conçue, implémentée, revue et livrée par des agents IA collaborant de façon asynchrone. Depuis août 2026, le projet est maintenu dans le même esprit — une session Claude principale unique travaille sur un registre des écarts de parité comportementale face au CLI TypeScript original, avec un responsable humain qui assure la revue et les fusions. Toujours zéro code de production écrit par des humains (~33 000 lignes aujourd'hui). La trace complète des décisions se trouve dans [`docs/project/`](docs/project/).

## Développement

### Prérequis

- Go 1.21+
- `golangci-lint` (optionnel, pour le linting)

### Compiler et tester

```bash
# Compiler
make build

# Exécuter tous les tests
make test

# Exécuter les tests avec rapport de couverture
make test-cover

# Vet
make vet

# Lint (nécessite golangci-lint)
make lint

# Compiler + tester + vet
make all
```

## Feuille de route

La construction initiale (avril 2026) a livré l'architecture complète à six couches en 9 jours. Depuis août 2026, le projet est en **mode maintenance** : le développement est piloté par un registre des écarts de parité comportementale ([`test/parity/cases.md`](test/parity/cases.md)) qui compare ce CLI à la version TypeScript originale — chaque écart fermé est verrouillé octet par octet par des tests, chaque divergence délibérée est enregistrée comme `waived` avec sa raison.

Jalons récents :

- **2026-09-05** — groupes CLI `mcp` (7 commandes) et `plugin` (8 commandes + sous-arbre `marketplace`) livrés, sortie identique octet par octet au CLI TS ; stubs visibles par l'utilisateur réduits de 42 à 27
- **2026-08-30** — parité du format de `--version` ; inventaire complet des sous-commandes face au CLI TS (§B11)
- **2026-08-29** — processus de mode maintenance livré : lignes rouges de dette appliquées par la CI, squelette de tests de parité

Le plan initial par phases (v0.2.0 → v1.0.0) est conservé, avec l'état d'achèvement de chaque phase, dans [`docs/ROADMAP.md`](docs/ROADMAP.md).

### État actuel

```
✅ Terminé :  Moteur central + streaming, TUI, 34 outils, 17 commandes slash (4 stubs),
              sous-commandes CLI mcp/plugin/auth/doctor, pipeline de permissions à 9 couches,
              compaction de contexte (snip/micro/auto), OAuth, persistance de session
🔧 Mode :     Maintenance — travail de parité piloté par le registre des écarts face au CLI TypeScript
📉 Dette :    56 TODO(dep) / 27 stubs visibles par l'utilisateur, lignes rouges à la baisse appliquées par la CI
🚀 Release :  v0.8.0, binaires pour 5 plateformes
```

## Contribuer

Les contributions sont les bienvenues ! Veuillez lire [CONTRIBUTING.md](CONTRIBUTING.md) avant de soumettre une pull request.

Checklist rapide :
- Forkez le dépôt et créez une branche de fonctionnalité
- Assurez-vous que `make test` et `make vet` passent
- Écrivez des tests pour les nouvelles fonctionnalités
- Suivez le style de code existant (exécutez `gofmt ./...`)
- Ouvrez une pull request en utilisant le template fourni

## Sécurité

Pour signaler une vulnérabilité de sécurité, veuillez consulter [SECURITY.md](SECURITY.md). **N'ouvrez pas** d'issue GitHub publique pour les bugs de sécurité.

## Licence

Ce projet est sous licence MIT — voir [LICENSE](LICENSE) pour les détails.

## Projets connexes

- [claude-code](https://github.com/anthropics/claude-code) — le CLI TypeScript original
- [anthropic-sdk-go](https://github.com/anthropics/anthropic-sdk-go) — SDK Go officiel pour l'API Anthropic
- [Model Context Protocol](https://modelcontextprotocol.io) — standard ouvert pour connecter l'IA aux outils
