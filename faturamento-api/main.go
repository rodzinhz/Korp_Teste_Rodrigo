package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// ---------- Modelos ----------

// ItemNota é o formato de entrada (o que o Angular manda ao criar a nota).
type ItemNota struct {
	ProdutoID  int     `json:"produto_id"`
	Descricao  string  `json:"descricao"`
	Quantidade int     `json:"quantidade"`
	Preco      float64 `json:"preco"` // preço unitário no momento da venda
}

// ItemDetalhe é o formato de saída (o que listamos de volta ao Angular).
// Agora vem gravado no próprio banco de faturamento — não dependemos mais
// de uma chamada HTTP ao estoque-api a cada listagem de notas. Isso resolve
// dois problemas do código anterior: preço perdido no cadastro da nota e
// fragilidade (lista de notas quebrando se o estoque-api estiver fora).
type ItemDetalhe struct {
	ProdutoID     int     `json:"produto_id"`
	Descricao     string  `json:"descricao"`
	Qtd           int     `json:"qtd"`
	PrecoUnitario float64 `json:"preco_unitario"`
	Total         float64 `json:"total"`
}

type Nota struct {
	ID         int           `json:"numero"`
	Status     string        `json:"status"`
	Cliente    string        `json:"cliente"`
	ValorTotal float64       `json:"valorTotal"`
	Data       string        `json:"data"`
	Itens      []ItemNota    `json:"itens"`    // usado só na criação (entrada)
	Detalhes   []ItemDetalhe `json:"detalhes"` // usado na listagem (saída)
}

type ErroAPI struct {
	Erro string `json:"erro"`
}

// ItemDescontado representa um item cujo estoque já foi descontado com
// sucesso durante o fechamento da nota. É um tipo nomeado (em vez de
// struct anônima) para poder ser usado tanto em fecharNota quanto em
// desfazerDescontos sem o compilador reclamar de tipos incompatíveis.
type ItemDescontado struct {
	ProdutoID  int
	Quantidade int
}

var db *sql.DB

const estoqueBaseURL = "http://localhost:8081"

// ---------- Infra ----------

func iniciarBanco() {
	var err error
	db, err = sql.Open("sqlite3", "./faturamento.db")
	if err != nil {
		log.Fatal("Erro ao abrir banco:", err)
	}

	db.Exec(`CREATE TABLE IF NOT EXISTS notas (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        status TEXT NOT NULL,
        cliente TEXT,
        valor_total REAL,
        data TEXT,
        idempotency_key TEXT UNIQUE
    )`)

	db.Exec(`CREATE TABLE IF NOT EXISTS itens_nota (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        nota_id INTEGER NOT NULL,
        produto_id INTEGER NOT NULL,
        descricao TEXT NOT NULL DEFAULT '',
        quantidade INTEGER NOT NULL,
        preco_unitario REAL NOT NULL DEFAULT 0
    )`)

	// Migrações leves para bancos já existentes de versões antigas.
	db.Exec(`ALTER TABLE notas ADD COLUMN idempotency_key TEXT`)
	db.Exec(`ALTER TABLE itens_nota ADD COLUMN descricao TEXT NOT NULL DEFAULT ''`)
	db.Exec(`ALTER TABLE itens_nota ADD COLUMN preco_unitario REAL NOT NULL DEFAULT 0`)

	// Tabela auxiliar para idempotência do fechamento/impressão de nota,
	// que não tem uma coluna própria em "notas" (é uma ação, não um recurso).
	db.Exec(`CREATE TABLE IF NOT EXISTS operacoes_idempotentes (
        chave TEXT PRIMARY KEY,
        resultado TEXT NOT NULL,
        criado_em TEXT NOT NULL
    )`)

	log.Println("Banco de faturamento iniciado!")
}

func responderErro(w http.ResponseWriter, status int, mensagem string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErroAPI{Erro: mensagem})
}

func responderJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}

func habilitarCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Idempotency-Key")
}

// ---------- Listagem / criação ----------

func listarNotas(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT id, status, cliente, valor_total, data FROM notas ORDER BY id DESC")
	if err != nil {
		responderErro(w, http.StatusInternalServerError, "Erro ao buscar notas")
		return
	}
	defer rows.Close()

	notas := []Nota{}

	for rows.Next() {
		var n Nota
		if err := rows.Scan(&n.ID, &n.Status, &n.Cliente, &n.ValorTotal, &n.Data); err != nil {
			continue
		}

		itemRows, err := db.Query(`
			SELECT produto_id, descricao, quantidade, preco_unitario
			FROM itens_nota WHERE nota_id = ?`, n.ID)
		if err == nil {
			n.Detalhes = []ItemDetalhe{}
			for itemRows.Next() {
				var it ItemDetalhe
				itemRows.Scan(&it.ProdutoID, &it.Descricao, &it.Qtd, &it.PrecoUnitario)
				it.Total = it.PrecoUnitario * float64(it.Qtd)
				n.Detalhes = append(n.Detalhes, it)
			}
			itemRows.Close()
		}

		notas = append(notas, n)
	}

	responderJSON(w, http.StatusOK, notas)
}

// criarNota grava a nota e seus itens já com o preço unitário informado
// pelo Angular no momento da montagem (evita depender de outro serviço
// para reconstruir o valor depois). Suporta idempotência: se o cabeçalho
// Idempotency-Key repetir uma chamada já processada, devolve a mesma nota
// em vez de criar uma duplicada (útil se o Angular reenviar por timeout).
func criarNota(w http.ResponseWriter, r *http.Request) {
	chaveIdemp := r.Header.Get("Idempotency-Key")

	if chaveIdemp != "" {
		var existenteID int
		err := db.QueryRow("SELECT id FROM notas WHERE idempotency_key = ?", chaveIdemp).Scan(&existenteID)
		if err == nil {
			// já processado antes: devolve a nota existente
			n := buscarNotaCompleta(existenteID)
			if n != nil {
				responderJSON(w, http.StatusOK, n)
				return
			}
		}
	}

	var n Nota
	if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
		responderErro(w, http.StatusBadRequest, "Erro nos dados da nota")
		return
	}
	if n.Cliente == "" || len(n.Itens) == 0 {
		responderErro(w, http.StatusBadRequest, "Informe o cliente e ao menos um item")
		return
	}

	tx, err := db.Begin()
	if err != nil {
		responderErro(w, http.StatusInternalServerError, "Erro ao iniciar transação")
		return
	}

	var valorTotal float64
	for _, item := range n.Itens {
		valorTotal += item.Preco * float64(item.Quantidade)
	}

	var idempKeyParam interface{}
	if chaveIdemp != "" {
		idempKeyParam = chaveIdemp
	} else {
		idempKeyParam = nil
	}

	result, err := tx.Exec(
		"INSERT INTO notas (status, cliente, valor_total, data, idempotency_key) VALUES (?, ?, ?, ?, ?)",
		"Aberta", n.Cliente, valorTotal, time.Now().Format("02/01/2006 15:04"), idempKeyParam,
	)
	if err != nil {
		tx.Rollback()
		responderErro(w, http.StatusInternalServerError, "Erro ao salvar nota")
		return
	}

	notaID, _ := result.LastInsertId()
	n.ID = int(notaID)

	for _, item := range n.Itens {
		_, err := tx.Exec(
			"INSERT INTO itens_nota (nota_id, produto_id, descricao, quantidade, preco_unitario) VALUES (?, ?, ?, ?, ?)",
			n.ID, item.ProdutoID, item.Descricao, item.Quantidade, item.Preco,
		)
		if err != nil {
			tx.Rollback()
			responderErro(w, http.StatusInternalServerError, "Erro ao salvar itens da nota")
			return
		}
	}

	if err := tx.Commit(); err != nil {
		responderErro(w, http.StatusInternalServerError, "Erro ao confirmar nota")
		return
	}

	nCompleta := buscarNotaCompleta(n.ID)
	responderJSON(w, http.StatusCreated, nCompleta)
}

func buscarNotaCompleta(id int) *Nota {
	var n Nota
	err := db.QueryRow("SELECT id, status, cliente, valor_total, data FROM notas WHERE id = ?", id).
		Scan(&n.ID, &n.Status, &n.Cliente, &n.ValorTotal, &n.Data)
	if err != nil {
		return nil
	}

	rows, err := db.Query(`
		SELECT produto_id, descricao, quantidade, preco_unitario
		FROM itens_nota WHERE nota_id = ?`, n.ID)
	if err == nil {
		n.Detalhes = []ItemDetalhe{}
		for rows.Next() {
			var it ItemDetalhe
			rows.Scan(&it.ProdutoID, &it.Descricao, &it.Qtd, &it.PrecoUnitario)
			it.Total = it.PrecoUnitario * float64(it.Qtd)
			n.Detalhes = append(n.Detalhes, it)
		}
		rows.Close()
	}

	return &n
}

func excluirNota(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		responderErro(w, http.StatusBadRequest, "ID inválido")
		return
	}

	var status string
	err = db.QueryRow("SELECT status FROM notas WHERE id = ?", id).Scan(&status)
	if err != nil {
		responderErro(w, http.StatusNotFound, "Nota não encontrada")
		return
	}

	if status == "Fechada" {
		rows, _ := db.Query("SELECT produto_id, quantidade FROM itens_nota WHERE nota_id = ?", id)
		for rows.Next() {
			var produtoID, qtd int
			rows.Scan(&produtoID, &qtd)

			body, _ := json.Marshal(map[string]int{
				"produto_id": produtoID,
				"quantidade": qtd,
			})
			http.Post(estoqueBaseURL+"/produtos/devolver", "application/json", bytes.NewBuffer(body))
		}
		rows.Close()
	}

	db.Exec("DELETE FROM itens_nota WHERE nota_id = ?", id)
	db.Exec("DELETE FROM notas WHERE id = ?", id)
	w.WriteHeader(http.StatusOK)
}

// ---------- Fechamento / impressão da nota ----------

// fecharNota é o ponto mais sensível do sistema: precisa descontar o
// estoque de vários produtos via chamadas HTTP a outro microsserviço.
// Duas coisas importantes acontecem aqui:
//
//  1. Compensação: se o desconto do 2º item falhar depois do 1º já ter
//     sido descontado com sucesso, devolvemos o que já foi descontado
//     (chamando /produtos/devolver) antes de reportar erro — assim a nota
//     não fica "meio fechada" e o estoque não fica inconsistente.
//
//  2. Idempotência: se o Angular reenviar o PUT (ex: por timeout de rede,
//     sem saber se a primeira chamada teve sucesso), usamos a chave do
//     header Idempotency-Key para não descontar o estoque duas vezes.
func fecharNota(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		responderErro(w, http.StatusBadRequest, "ID não fornecido")
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		responderErro(w, http.StatusBadRequest, "ID inválido")
		return
	}

	chaveIdemp := r.Header.Get("Idempotency-Key")
	if chaveIdemp != "" {
		var resultadoSalvo string
		err := db.QueryRow("SELECT resultado FROM operacoes_idempotentes WHERE chave = ?", chaveIdemp).Scan(&resultadoSalvo)
		if err == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(resultadoSalvo))
			return
		}
	}

	var status string
	if err := db.QueryRow("SELECT status FROM notas WHERE id = ?", id).Scan(&status); err != nil {
		responderErro(w, http.StatusNotFound, "Nota não encontrada")
		return
	}
	if status != "Aberta" {
		responderErro(w, http.StatusConflict, "Somente notas com status Aberta podem ser impressas")
		return
	}

	rows, _ := db.Query("SELECT produto_id, quantidade FROM itens_nota WHERE nota_id = ?", id)
	type item struct{ produtoID, quantidade int }
	var itens []item
	for rows.Next() {
		var it item
		rows.Scan(&it.produtoID, &it.quantidade)
		itens = append(itens, it)
	}
	rows.Close()

	var descontados []ItemDescontado
	for _, it := range itens {
		body, _ := json.Marshal(map[string]int{
			"produto_id": it.produtoID,
			"quantidade": it.quantidade,
		})

		resp, err := http.Post(estoqueBaseURL+"/produtos/descontar", "application/json", bytes.NewBuffer(body))
		if err != nil {
			desfazerDescontos(descontados)
			responderErro(w, http.StatusServiceUnavailable, "Serviço de estoque indisponível. Nenhuma alteração foi aplicada.")
			return
		}
		sucesso := resp.StatusCode == http.StatusOK
		resp.Body.Close()

		if !sucesso {
			desfazerDescontos(descontados)
			responderErro(w, http.StatusConflict, "Não foi possível descontar o estoque de um dos itens (saldo insuficiente). Nenhuma alteração foi aplicada.")
			return
		}

		descontados = append(descontados, ItemDescontado{ProdutoID: it.produtoID, Quantidade: it.quantidade})
	}

	if _, err := db.Exec("UPDATE notas SET status = ? WHERE id = ?", "Fechada", id); err != nil {
		desfazerDescontos(descontados)
		responderErro(w, http.StatusInternalServerError, "Erro ao atualizar nota; estoque foi revertido")
		return
	}

	nCompleta := buscarNotaCompleta(id)
	respostaBytes, _ := json.Marshal(nCompleta)

	if chaveIdemp != "" {
		db.Exec(
			"INSERT OR REPLACE INTO operacoes_idempotentes (chave, resultado, criado_em) VALUES (?, ?, ?)",
			chaveIdemp, string(respostaBytes), time.Now().Format(time.RFC3339),
		)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(respostaBytes)
}

// desfazerDescontos chama /produtos/devolver para cada item já descontado
// com sucesso, revertendo o estoque em caso de falha no meio do processo.
func desfazerDescontos(itens []ItemDescontado) {
	for _, it := range itens {
		body, _ := json.Marshal(map[string]int{
			"produto_id": it.ProdutoID,
			"quantidade": it.Quantidade,
		})
		resp, err := http.Post(estoqueBaseURL+"/produtos/devolver", "application/json", bytes.NewBuffer(body))
		if err == nil {
			resp.Body.Close()
		}
	}
}

// ---------- main ----------

func main() {
	iniciarBanco()

	http.HandleFunc("/notas", func(w http.ResponseWriter, r *http.Request) {
		habilitarCORS(w)
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		switch r.Method {
		case "GET":
			listarNotas(w, r)
		case "POST":
			criarNota(w, r)
		case "DELETE":
			excluirNota(w, r)
		case "PUT":
			fecharNota(w, r)
		default:
			responderErro(w, http.StatusMethodNotAllowed, "Método não permitido")
		}
	})

	log.Println("Faturamento service rodando na porta 8082...")
	log.Fatal(http.ListenAndServe(":8082", nil))
}
