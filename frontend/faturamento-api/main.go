package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type ItemDetalhe struct {
	Descricao     string  `json:"nome"`
	Qtd           int     `json:"qtd"`
	Total         float64 `json:"total"`
	PrecoUnitario float64 `json:"preco_unitario"`
}

type ItemNota struct {
	ProdutoID  int `json:"produto_id"`
	Quantidade int `json:"quantidade"`
	ID         int `json:"id"`
	NOTAID     int `json:"nota_id"`
}

type Nota struct {
	ID         int           `json:"numero"`
	Status     string        `json:"status"`
	Cliente    string        `json:"cliente"`
	ValorTotal float64       `json:"valorTotal"`
	Data       string        `json:"data"`
	Itens      []ItemNota    `json:"itens"`
	Detalhes   []ItemDetalhe `json:"detalhes"`
}

var db *sql.DB

func iniciarBanco() {
	var err error
	db, err = sql.Open("sqlite3", "./faturamento.db")
	if err != nil {
		log.Fatal("Erro ao abrir banco:", err)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS notas (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        status TEXT NOT NULL,
        cliente TEXT,
        valor_total REAL,
        data TEXT
    )`)

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS itens_nota (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        nota_id INTEGER NOT NULL,
        produto_id INTEGER NOT NULL,
        quantidade INTEGER NOT NULL
    )`)
	log.Println("Banco de faturamento iniciado!")
}

func listarNotas(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT id, status, cliente, valor_total, data FROM notas ORDER BY id DESC")
	if err != nil {
		http.Error(w, "Erro ao buscar notas", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var notas []Nota

	for rows.Next() {
		var n Nota
		rows.Scan(&n.ID, &n.Status, &n.Cliente, &n.ValorTotal, &n.Data)

		itemRows, err := db.Query(`
    SELECT produto_id, quantidade 
    FROM itens_nota 
    WHERE nota_id = ?`, n.ID)

		if err == nil {
			for itemRows.Next() {
				var produtoID, qtd int
				itemRows.Scan(&produtoID, &qtd)

				resp, err := http.Get(fmt.Sprintf("http://localhost:8081/produtos/%d", produtoID))
				if err == nil {
					var p map[string]interface{}
					json.NewDecoder(resp.Body).Decode(&p)
					resp.Body.Close()

					preco, _ := p["preco"].(float64)

					n.Detalhes = append(n.Detalhes, ItemDetalhe{
						Descricao:     fmt.Sprintf("%v", p["descricao"]),
						Qtd:           qtd,
						Total:         preco * float64(qtd),
						PrecoUnitario: preco,
					})
				}
			}
			itemRows.Close()
		}

		notas = append(notas, n)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(notas)
}

func criarNota(w http.ResponseWriter, r *http.Request) {
	var n Nota
	if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
		http.Error(w, "Erro nos dados", http.StatusBadRequest)
		return
	}

	result, err := db.Exec(
		"INSERT INTO notas (status, cliente, valor_total, data) VALUES (?, ?, ?, ?)",
		"Aberta", n.Cliente, n.ValorTotal, time.Now().Format("02/01/2006 15:04"),
	)

	if err != nil {
		http.Error(w, "Erro ao salvar nota", http.StatusInternalServerError)
		return
	}

	notaID, _ := result.LastInsertId()
	n.ID = int(notaID)

	for _, item := range n.Itens {
		db.Exec("INSERT INTO itens_nota (nota_id, produto_id, quantidade) VALUES (?, ?, ?)",
			n.ID, item.ProdutoID, item.Quantidade)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(n)
}

func excluirNota(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "ID inválido", http.StatusBadRequest)
		return
	}

	var status string
	err = db.QueryRow("SELECT status FROM notas WHERE id = ?", id).Scan(&status)
	if err != nil {
		http.Error(w, "Nota não encontrada", http.StatusNotFound)
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

			http.Post(
				"http://localhost:8081/produtos/devolver",
				"application/json",
				bytes.NewBuffer(body),
			)
		}
		rows.Close()
	}

	db.Exec("DELETE FROM itens_nota WHERE nota_id = ?", id)
	db.Exec("DELETE FROM notas WHERE id = ?", id)
	w.WriteHeader(http.StatusOK)
}

func fecharNota(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		http.Error(w, "ID não fornecido", http.StatusBadRequest)
		return
	}

	rows, _ := db.Query("SELECT produto_id, quantidade FROM itens_nota WHERE nota_id = ?", idStr)
	defer rows.Close()

	for rows.Next() {
		var item ItemNota
		rows.Scan(&item.ProdutoID, &item.Quantidade)

		body, _ := json.Marshal(map[string]int{
			"produto_id": item.ProdutoID,
			"quantidade": item.Quantidade,
		})

		resp, _ := http.Post(
			"http://localhost:8081/produtos/descontar",
			"application/json",
			bytes.NewBuffer(body),
		)
		if resp != nil {
			resp.Body.Close()
		}
	}

	_, err := db.Exec("UPDATE notas SET status = ? WHERE id = ?", "Fechada", idStr)
	if err != nil {
		http.Error(w, "Erro ao atualizar banco", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func habilitarCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

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
			http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		}
	})

	log.Println("Faturamento service rodando na porta 8082...")
	log.Fatal(http.ListenAndServe(":8082", nil))
}
