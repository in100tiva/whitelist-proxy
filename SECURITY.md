# Política de segurança

Este projeto é um proxy local de filtragem de tráfego. Vulnerabilidades aqui
podem significar **bypass de política de rede** em máquinas de terceiros, então
levamos os relatos a sério e pedimos que não sejam divulgados publicamente
antes da correção.

## Versões suportadas

| Versão | Suporte |
|--------|---------|
| `1.x` (branch `main`) | ✅ Correções de segurança e bugs |
| `< 1.0` (pré-release) | ❌ Sem suporte — atualize para a release mais recente |

## Como relatar

**Não abra issue ou discussion pública para falhas de segurança.**

Use o canal privado do GitHub:

1. Acesse <https://github.com/in100tiva/whitelist-proxy/security/advisories/new>
2. Descreva o problema com o máximo de detalhe possível.

Inclua, se puder:

- versão do binário (`proxy.exe version`) e do Windows;
- modo de execução (serviço ou `proxy.exe run`);
- passos para reproduzir e o impacto que você consegue demonstrar;
- trecho de log (`logs/proxy-AAAA-MM-DD.log`) **com domínios internos removidos**.

### O que esperar

| Etapa | Prazo alvo |
|-------|-----------|
| Confirmação de recebimento | até 3 dias úteis |
| Triagem e avaliação de impacto | até 7 dias úteis |
| Correção + release | depende da criticidade; falhas de bypass recebem prioridade |

Você é creditado no advisory (ou fica anônimo, se preferir). Não há programa de
recompensa financeira.

## Escopo

**Em escopo** (exemplos):

- bypass da whitelist sem privilégio de administrador;
- acesso à API administrativa (`127.0.0.1:8081`) sem o token;
- vazamento do `admin.token` ou das regras para outros processos/usuários sem privilégio;
- execução de código, path traversal ou DoS a partir de requisições do proxy ou da UI;
- elevação de privilégio pelo `install.ps1` ou pelo registro do serviço.

**Fora de escopo** (documentado como limitação conhecida no README):

- usuário com privilégio de administrador local desligando o proxy do sistema
  (`HKCU`) ou o serviço;
- tráfego QUIC/HTTP3 e DoH que não passam pelo proxy TCP — ver
  [Limitações conhecidas](README.md#limitações-conhecidas);
- aplicações que ignoram a configuração de proxy do sistema;
- necessidade de privilégio administrativo para instalar/remover o serviço.

## Boas práticas de quem opera

- Rode o serviço com a conta de serviço padrão — nunca com credenciais de domínio.
- Restrinja as permissões NTFS de `C:\Program Files\WhitelistProxy\` (o `admin.token`
  é gerado lá na primeira execução).
- A UI e a API escutam apenas em `127.0.0.1`; não exponha essas portas na rede.
- Combine o proxy com regras de firewall de saída quando a política precisar ser
  realmente inescapável (ver roadmap de fase 2).
