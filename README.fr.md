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

### Entièrement construit par des agents IA — zéro code écrit par des humains

> **Aucun humain n'a écrit une seule ligne de code de production dans ce dépôt.**

L'ensemble du projet — conception de l'architecture, documents de conception détaillés, implémentation parallèle, revue de code, QA et tests d'intégration — a été produit par **9 agents IA Claude** collaborant dans un workflow multi-agents structuré :

```
Agent PM          →  plan de projet, jalons, planification des tâches
Agent Tech Lead   →  conception d'architecture, revue de documents de conception, revue de code
Agent-Infra       →  couche d'infrastructure (types, configuration, état, session)
Agent-Services    →  couche de services (client API, OAuth, MCP, compaction)
Agent-Core        →  moteur central (boucle LLM, dispatch d'outils, coordinateur)
Agent-Tools       →  couche d'outils (fichier, shell, recherche, web — 18 outils)
Agent-TUI         →  couche UI (Bubble Tea MVU, thèmes, mode Vim)
Agent-CLI         →  couche d'entrée (Cobra CLI, DI, phases de bootstrap)
Agent QA          →  stratégie de test, acceptation par couche, tests d'intégration
```

Résultat : ~**7 000 lignes de code de production + suite de tests complète**, avec `go test -race ./...` qui passe.

---

## Fonctionnalités

- **TUI interactive** — Interface terminal complète construite avec [Bubble Tea](https://github.com/charmbracelet/bubbletea), avec thèmes sombre/clair
- **Utilisation d'outils agentiques** — Lecture/écriture de fichiers, exécution shell, recherche, et plus, le tout médié par une couche de permissions
- **Coordination multi-agents** — Lance des sous-agents en arrière-plan pour des tâches parallèles
- **Support MCP** — Connecte des outils externes via le [Model Context Protocol](https://modelcontextprotocol.io)
- **Mémoire CLAUDE.md** — Charge automatiquement le contexte du projet depuis les fichiers `CLAUDE.md` dans l'arborescence
- **Gestion des sessions** — Reprend les conversations précédentes ; compacte automatiquement les historiques longs
- **Mode Vim** — Raccourcis clavier Vim optionnels dans la zone de saisie
- **Authentification OAuth + clé API** — Connectez-vous avec OAuth Anthropic ou fournissez une `ANTHROPIC_API_KEY`
- **18 commandes slash intégrées** — `/help`, `/clear`, `/compact`, `/commit`, `/diff`, `/review`, `/mcp`, et plus
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
```

**OAuth (recommandé pour une utilisation interactive) :**

```bash
claude /config    # ouvre le flux OAuth dans votre navigateur
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

### Commandes slash

Tapez `/` dans la zone de saisie pour voir toutes les commandes disponibles :

| Commande | Description |
|----------|-------------|
| `/help` | Afficher toutes les commandes |
| `/clear` | Effacer l'historique de conversation |
| `/compact` | Résumer l'historique pour réduire l'utilisation du contexte |
| `/exit` | Quitter Claude Code |
| `/model` | Changer de modèle Claude |
| `/theme` | Basculer entre thème sombre/clair |
| `/vim` | Basculer le mode Vim |
| `/commit` | Générer un message de commit git |
| `/review` | Examiner les changements récents |
| `/diff` | Afficher le diff actuel |
| `/mcp` | Gérer les serveurs MCP |
| `/memory` | Afficher les fichiers CLAUDE.md chargés |
| `/session` | Afficher les informations de session |
| `/status` | Afficher le statut API/connexion |
| `/cost` | Afficher l'utilisation des tokens et le coût estimé |

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

Claude Code Go est actuellement à environ **65%** de parité fonctionnelle avec la version TypeScript originale. Voici notre plan par phases pour atteindre la v1.0 :

| Phase | Version | Objectifs clés | Délai |
|-------|---------|----------------|-------|
| **Phase 1** | v0.2.0 | 🔒 Intégration du système de permissions, connexion du système Hook, base de couverture de tests, renforcement CI | +3 semaines |
| **Phase 2** | v0.3.0 | 🔧 Compléter les 22 outils (actuellement 11), sous-commandes CLI, améliorations des commandes slash, outil Agent | +3 semaines |
| **Phase 3** | v0.4.0 | 🌐 Fournisseurs AWS Bedrock & GCP Vertex, transport MCP WebSocket, système de plugins, Feature Flags | +4 semaines |
| **Phase 4** | v0.5.0 | 🚀 Intégration LSP, mode Remote/Server, entrée vocale, mode Vim, Extended Thinking, suivi des coûts | +4 semaines |
| **Phase 5** | v1.0.0 | 🎯 Optimisation des performances, audit de sécurité, documentation complète, publication multiplateforme | +2 semaines |

### État actuel

```
Achèvement : ████████████░░░░░░░░ 65%

✅ Terminé : Moteur central, TUI, client API (Direct + OpenAI), compaction de contexte,
             OAuth, persistance de session, 11 outils, 14 commandes slash
⚠️  En cours : Fournisseurs Bedrock/Vertex, MCP WebSocket, outils et commandes restants
❌ À faire : Connexion des permissions, connexion des Hooks, LSP, système de plugins, mode Remote
```

📋 Consultez la **[feuille de route complète](docs/ROADMAP.md)** pour les détails des tâches, les diagrammes d'architecture et les critères d'achèvement.

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
