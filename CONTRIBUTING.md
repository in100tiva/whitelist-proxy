# Como contribuir

Obrigado pelo interesse! Este guia cobre o essencial para mandar uma contribuição
que entre sem rodada extra de revisão.

## Antes de começar

- **Bug?** Abra uma [issue](.github/ISSUE_TEMPLATE/bug_report.yml) ou, se for falha
  de segurança, siga a [política de segurança](SECURITY.md) (canal privado).
- **Ideia?** Abra uma [issue de ideia](.github/ISSUE_TEMPLATE/feature_request.yml)
  ou passe nas [Discussions](https://github.com/in100tiva/whitelist-proxy/discussions).
  Mudanças de comportamento da whitelist merecem discussão antes do código.
- **Dúvida de uso?** Veja o [SUPPORT.md](SUPPORT.md).

## Ambiente

| Item | Versão |
|------|--------|
| Go | 1.22+ (`go.mod`) |
| Windows | para testar serviço, registro e `install.ps1` |
| Linux/macOS | suficiente para desenvolver e rodar `go test ./...` |

O projeto compila em qualquer SO: as partes específicas de Windows ficam atrás
de build tags (`service_windows.go` / `service_other.go`, `sysproxy/windows.go` /
`sysproxy/other.go`). Mantenha essa separação — código novo que só existe no
Windows precisa de um stub no arquivo `other.go` correspondente.

## Fluxo de trabalho

```sh
git clone https://github.com/in100tiva/whitelist-proxy.git
cd whitelist-proxy
go build ./cmd/proxy
go test ./... -race
```

Ciclo de desenvolvimento em foreground (sem registrar serviço):

```sh
go run ./cmd/proxy run
```

Abra a UI em <http://127.0.0.1:8081/> usando a URL com token que o comando
`go run ./cmd/proxy ui` imprime.

## Requisitos de qualidade

Todo pull request precisa passar no que a CI executa:

```sh
gofmt -l .            # deve sair vazio
go vet ./...
go build ./...
go test ./... -race
GOOS=windows GOARCH=amd64 go build -o /dev/null ./cmd/proxy
```

Regras:

- **Sem dependência nova sem justificativa** no PR. A biblioteca padrão dá conta
  da maior parte; hoje a única dependência é `golang.org/x/sys`.
- **Testes para lógica testável.** `internal/filter` e o parser de SNI
  (`internal/proxy/sni.go`) são cobertos por testes puros/loopback — mudanças
  ali exigem teste novo ou ajuste justificado.
- **Nada de segredo em commit.** `admin.token`, `whitelist.json` de cliente e
  `logs/` são ignorados pelo `.gitignore`; continue assim.
- **UI sem dependência de CDN.** O HTML/CSS/JS é embutido via `embed.FS` e precisa
  funcionar offline — sem `fetch` externo, sem framework baixado em runtime.
- **Código e comentários em português**, seguindo o estilo do repositório.

## Commits e branches

- Branch a partir de `main`: `feat/nome-curto`, `fix/nome-curto`, `docs/...`.
- Mensagens no padrão [Conventional Commits](https://www.conventionalcommits.org/pt-br/),
  com assunto em português e imperativo:

  ```text
  feat(filter): aceitar curinga no meio do padrão
  fix(admin): impedir cache do index quando o token muda
  docs(readme): explicar o whitelist.json de exemplo
  ```

- Pull requests são integrados por **squash merge**; o título do PR vira a
  mensagem final no `main`, então capriche nele.

## Revisão

- Um revisor é atribuído automaticamente via [CODEOWNERS](.github/CODEOWNERS).
- PRs que mudam comportamento de filtro ou formato do `whitelist.json` devem
  atualizar README e CHANGELOG no mesmo PR.
- Feedback é sempre sobre o código, nunca sobre a pessoa — veja o
  [Código de Conduta](CODE_OF_CONDUCT.md).

## Licença

Ao contribuir, você concorda que sua contribuição é licenciada sob a
[MIT](LICENSE), como o restante do projeto.
