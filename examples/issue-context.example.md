# DEMO-123

## Título
Corrigir login social

## Status
Status: In Progress | Prioridade: High | Sprint: Sprint 42

Story Points: 3

Labels: backend, auth

## Descrição
O login via Google e GitHub falha intermitentemente após a expiração do token.
Precisamos renovar o token de forma transparente.

## Critérios de aceite
- Usuário permanece logado após a expiração do token de acesso
- Falha de renovação redireciona para a tela de login com mensagem clara

## Comentários
- **Ana Souza**: O problema parece estar no refresh token; ver service OAuthSession.

## Projeto
Diretório: `/Users/dev/projects/demo`

Estrutura principal:
```
app/
composer.json
docs/
package.json
resources/
routes/
src/
```

Documentação (`docs/`):
- docs/architecture.md
- docs/auth.md

## README
# Demo
Laravel + React + PostgreSQL. Plataforma de eventos.

## Git
Branch atual: `demo-123-corrigir-login-social`

Últimos commits:
- a1b2c3d feat: adiciona provider GitHub
- d4e5f6a fix: ajusta callback do Google

---
_Gerado pelo SprintMate. Issue: https://empresa.atlassian.net/browse/DEMO-123_
