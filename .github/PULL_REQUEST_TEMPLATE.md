# Pull request

## O que muda

<!-- Uma frase explicando o objetivo da mudança. -->

## Tipo de mudança

- [ ] Bugfix
- [ ] Nova funcionalidade
- [ ] Mudança de comportamento / regra de filtro
- [ ] Documentação
- [ ] CI, build ou dependências
- [ ] Refatoração sem mudança de comportamento

## Como foi testado

<!-- Cole os comandos e o resultado resumido. Exemplos:
     go vet ./...
     go test ./... -race
     teste manual: curl -x http://127.0.0.1:8080 https://dominio-fora-da-lista
-->

## Checklist

- [ ] `gofmt -l .` não lista arquivos
- [ ] `go vet ./...` passa
- [ ] `go test ./...` passa
- [ ] Testei o caminho Windows (serviço e/ou `proxy.exe run`), quando aplicável
- [ ] Atualizei o `README.md` / `CHANGELOG.md` quando o comportamento mudou
- [ ] Não incluí segredos, tokens (`admin.token`), logs de cliente ou dados internos

## Issue relacionada

<!-- Closes #123 -->
