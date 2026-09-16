# Send Push Lambda

AWS Lambda que consome uma fila SQS de push notifications e envia via Firebase Cloud Messaging (FCM). Mensagens que falham (JSON inválido ou erro do FCM) voltam pra fila e, depois de algumas tentativas, caem numa dead-letter queue — ver `infra/modules/sqs-lambda-service`.

## Como funciona

A fila SQS aciona a Lambda via event source mapping (com `ReportBatchItemFailures`, então uma falha numa mensagem do lote não força reprocessar o lote inteiro). Pra cada mensagem:

1. Faz parse do envelope JSON
2. Chama `messaging.Send` no FCM com o token do dispositivo, título, corpo, canal Android e dados extras
3. Se falhar, reporta a mensagem como falha do lote (`BatchItemFailures`) em vez de derrubar a invocação inteira

## Contrato da mensagem

Corpo da mensagem SQS (JSON) — um envelope genérico, sem conhecimento de domínio (dose reminder, share invite, ...). Quem decide o canal e monta `data` é quem publica na fila:

```json
{
  "token": "<fcm device token>",
  "title": "...",
  "body": "...",
  "android_channel": "cycle",
  "data": { "deepLink": "...", "notificationId": "..." }
}
```

| Campo | Descrição |
|---|---|
| `token` | Device token do FCM |
| `title` / `body` | Conteúdo da notificação |
| `android_channel` | Canal de notificação Android (ex: `cycle`, `share`) |
| `data` | Payload extra entregue ao app (deep link, IDs, etc.) |

## Deploy

Push para `main` dispara o GitHub Actions, que builda o binário Go (`GOARCH=arm64`) e atualiza o código da Lambda.

## Environment variables

| Variável | Descrição |
|---|---|
| `FCM_CREDENTIALS_PATH` | Caminho do parâmetro SSM (SecureString) com o JSON da service account do Firebase |

## SSM Parameters

Antes do primeiro deploy real, preencha o parâmetro no AWS Parameter Store (o Terraform cria só o placeholder `"CHANGEME"`):

```bash
aws ssm put-parameter \
  --name /cyclolab/dev/fcm-credentials \
  --type SecureString --value "$(cat service-account.json)" --overwrite
```
