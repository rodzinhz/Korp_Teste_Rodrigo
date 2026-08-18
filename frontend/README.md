Korp Flow — Teste Técnico (Rodrigo)

Sistema de emissão de Notas Fiscais desenvolvido para o teste técnico da Korp ERP, com arquitetura de microsserviços, tratamento de falhas e persistência real em banco de dados.

Arquitetura

O sistema é dividido em três partes independentes:

korp-project/
├── estoque-api/       (Go, porta 8081) — CRUD de produtos e controle de saldo
├── faturamento-api/   (Go, porta 8082) — criação e fechamento de notas fiscais
└── frontend/           (Angular, porta 4200) — interface web
Backend: dois microsserviços em Go, cada um com seu próprio banco SQLite (.db separado por serviço).
Frontend: Angular (standalone components), consumindo os dois backends via HTTP.
Requisitos obrigatórios atendidos

✅ Arquitetura de microsserviços (estoque e faturamento, independentes)
✅ Persistência real em banco de dados (SQLite, um arquivo por serviço)
✅ Tratamento de falhas: se um microsserviço cair, o outro continua funcionando normalmente e o frontend exibe um aviso claro na tela (banner de erro), sem travar e sem corromper dados. Assim que o serviço volta, o sistema opera normalmente de novo — testado derrubando o estoque-api no meio do fechamento de uma nota.

Diferenciais implementados (requisitos opcionais)
Concorrência: o desconto de saldo é feito de forma atômica direto no SQL (UPDATE produtos SET saldo = saldo - ? WHERE id = ? AND saldo >= ?), evitando que duas notas descontem o mesmo saldo ao mesmo tempo sem precisar de lock manual em Go.
Idempotência: as operações de criar e fechar/imprimir nota aceitam um header Idempotency-Key. Se a mesma chave for reenviada (ex: timeout e nova tentativa), o backend devolve o resultado já processado em vez de duplicar a operação.
Compensação em falha parcial: se o desconto de um item falhar depois de outro item já ter sido descontado, o sistema desfaz automaticamente o que já tinha sido descontado antes de reportar o erro — a nota nunca fica "meio fechada".
Uso de IA: botão "sugerir com IA" na tela de produtos. O usuário digita algo curto (ex: "parafuso 6mm") e a API do Google Gemini sugere uma descrição comercial e uma categoria, preenchendo os campos automaticamente.
Como rodar o projeto
Pré-requisitos
Go instalado
Node.js e Angular CLI instalados
Uma chave de API do Google Gemini (para o recurso de sugestão por IA)
1. Subir o estoque-api (porta 8081)
powershell
cd estoque-api
$env:GEMINI_API_KEY = "sua_chave_aqui"
go run main.go
2. Subir o faturamento-api (porta 8082)
powershell
cd faturamento-api
go run main.go
3. Subir o frontend Angular (porta 4200)
powershell
cd frontend
npm install
ng serve

Depois é só acessar http://localhost:4200 no navegador.

Importante: a chave da API do Gemini nunca é gravada no código-fonte — ela é lida via variável de ambiente GEMINI_API_KEY. Se ela não estiver configurada, o endpoint de sugestão por IA retorna um erro tratado em vez de quebrar o serviço.

Nota sobre o Gemini: o tier gratuito da API tem uma cota diária limitada de requisições. Se o limite for atingido, o backend retorna um erro tratado (API de IA retornou erro) e o restante do sistema continua funcionando normalmente — a sugestão por IA é um recurso opcional/diferencial, não uma dependência crítica do fluxo principal.

Testando o cenário de falha de microsserviço
Suba os três serviços normalmente.
Crie uma nota fiscal com um ou mais itens (deixe em status "Aberta").
Derrube o estoque-api (Ctrl+C no terminal dele).
Tente fechar/imprimir a nota — o sistema exibirá um banner de erro informando que o serviço de estoque está indisponível, sem aplicar nenhuma alteração parcial.
Suba o estoque-api novamente e tente fechar a nota de novo — o fechamento é concluído normalmente, confirmando a recuperação do sistema.
Stack
Backend: Go (sem frameworks, apenas biblioteca padrão), SQLite
Frontend: Angular (standalone components), RxJS
IA: Google Gemini API