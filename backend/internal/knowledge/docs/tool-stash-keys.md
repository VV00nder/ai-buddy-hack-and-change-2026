---
id: tool-stash-keys
title: SSH-ключи в Stash
audience: all
team: all
tags: stash, ssh, ключ, ключи, git, репозиторий, bitbucket
---
## Как добавить ключ
1. Сгенерируй ключ: `ssh-keygen -t ed25519 -C "твой-логин"`.
2. Скопируй публичную часть `~/.ssh/id_ed25519.pub`.
3. В Stash: профиль → SSH keys → Add key.
4. Проверь клонированием репозитория команды.

Нужен доступ «Stash: репозитории команды» — запроси его через AI-buddy.
