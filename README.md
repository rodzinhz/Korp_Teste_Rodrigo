### Sobre o Projeto
Sistema web de gerenciamento de produtos e emissão de notas fiscais, desenvolvido como teste técnico para a Korp ERP. A aplicação é composta por um frontend em Angular e dois microsserviços em Go, comunicando-se via HTTP/JSON com banco de dados SQLite.

---

### Tecnologias Utilizadas

**Frontend**
- Angular 17+ (standalone components)
- TypeScript
- RxJS (HttpClient + Observable)
- HTML + CSS

**Backend**
- Go (Golang) — linguagem principal dos microsserviços
- `net/http` — servidor HTTP nativo do Go
- `database/sql` + `go-sqlite3` — banco de dados
- SQLite — persistência dos dados em arquivo

**Ferramentas**
- Git + GitHub — versionamento
- TDM-GCC — compilador C necessário pro driver SQLite
- Node.js + Angular CLI — ambiente do frontend

---

### Arquitetura

```
Frontend Angular (porta 4200)
        ↓
estoque-api  (porta 8081) → estoque.db
faturamento-api (porta 8082) → faturamento.db
        ↓
estoque-api (desconto de saldo ao imprimir nota)
```

---

### Como Rodar o Projeto

**Pré-requisitos:**
- Go instalado
- Node.js + Angular CLI instalados
- TDM-GCC instalado (compilador C)

**1. Estoque Service**
```bash
cd estoque-api
go run main.go
# Rodando em http://localhost:8081
```

**2. Faturamento Service**
```bash
cd faturamento-api
go run main.go
# Rodando em http://localhost:8082
```

**3. Frontend Angular**
```bash
cd frontend
npm install
ng serve
# Acesse http://localhost:4200
```

---

### Funcionalidades

**Cadastro de Produtos**
- Código, descrição, preço e saldo
- Listagem e exclusão de produtos
- Dados persistidos no banco SQLite

**Notas Fiscais**
- Criação de nota com múltiplos produtos
- Status: Aberta ou Fechada
- Numeração sequencial automática

**Impressão de Notas**
- Botão de imprimir visível na tela
- Ao imprimir: status muda para Fechada
- Saldo dos produtos atualizado automaticamente
- Bloqueio de impressão para notas já fechadas
- Tratamento de falha: se o estoque-api estiver fora do ar, o usuário recebe mensagem de erro

---

### Microsserviços

**estoque-api**
- `GET /produtos` → lista produtos
- `POST /produtos` → cadastra produto
- `POST /produtos/descontar` → desconta saldo
- `DELETE /produtos/:id` → exclui produto

**faturamento-api**
- `GET /notas` → lista notas
- `POST /notas` → cria nota
- `PUT /notas?id=X` → imprime e fecha nota
- `DELETE /notas?id=X` → exclui nota

---

### Detalhamento Técnico

**Ciclos de vida Angular utilizados:**
- `ngOnInit` — carrega produtos e notas quando a tela abre

**RxJS:**
- Usado via `HttpClient` — cada requisição retorna um `Observable`
- `.subscribe()` usado para reagir às respostas assíncronas do backend

**Gerenciamento de dependências Go:**
- `go.mod` e `go.sum` — padrão nativo do Go
- `go get` para instalar bibliotecas externas

**Tratamento de erros no backend:**
- Todo endpoint verifica erros e retorna códigos HTTP adequados (400, 404, 500, 503)
- Se o estoque-api falhar durante a impressão, o faturamento-api retorna 503 e o Angular exibe mensagem ao usuário

**Frameworks Go utilizados:**
- Apenas a biblioteca padrão (`net/http`, `database/sql`, `encoding/json`)
