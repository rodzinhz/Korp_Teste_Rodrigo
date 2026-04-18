package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

type Produto struct {
	ID        int     `json:"id"`
	Codigo    string  `json:"codigo"`
	Descricao string  `json:"descricao"`
	Preco     float64 `json:"preco"`
	Saldo     int     `json:"saldo"`
}

var db *sql.DB

func iniciarBanco() {
	var err error

	db, err = sql.Open("sqlite3", "./estoque.db")
	if err != nil {
		log.Fatal("Erro ao abrir banco:", err)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS produtos (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		codigo TEXT NOT NULL,
		descricao TEXT NOT NULL,
		preco REAL NOT NULL,
		saldo INTEGER NOT NULL
	)`)
	if err != nil {
		log.Fatal("Erro ao criar tabela:", err)
	}

	log.Println("Banco de dados iniciado!")
}

func listarProdutos(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT id, codigo, descricao, preco, saldo FROM produtos")
	if err != nil {
		http.Error(w, "Erro ao buscar produtos", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var produtos []Produto
	for rows.Next() {
		var p Produto
		rows.Scan(&p.ID, &p.Codigo, &p.Descricao, &p.Preco, &p.Saldo)
		produtos = append(produtos, p)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(produtos)
}

func criarProduto(w http.ResponseWriter, r *http.Request) {
	var p Produto

	err := json.NewDecoder(r.Body).Decode(&p)
	if err != nil {
		http.Error(w, "Erro ao processar os dados do produto", http.StatusBadRequest)
		log.Println("Erro ao decodificar produto:", err)
		return
	}

	result, err := db.Exec(
		"INSERT INTO produtos (codigo, descricao, preco, saldo) VALUES (?, ?, ?, ?)",
		p.Codigo, p.Descricao, p.Preco, p.Saldo,
	)
	if err != nil {
		http.Error(w, "Erro ao salvar produto", http.StatusInternalServerError)
		log.Println("Erro ao inserir produto:", err)
		return
	}

	id, _ := result.LastInsertId()
	p.ID = int(id)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p)
}

func descontarSaldo(w http.ResponseWriter, r *http.Request) {
	var desconto struct {
		ProdutoID  int `json:"produto_id"`
		Quantidade int `json:"quantidade"`
	}

	err := json.NewDecoder(r.Body).Decode(&desconto)
	if err != nil {
		http.Error(w, "Erro ao processar desconto", http.StatusBadRequest)
		return
	}

	var p Produto
	err = db.QueryRow(
		"SELECT id, codigo, descricao, preco, saldo FROM produtos WHERE id = ?",
		desconto.ProdutoID,
	).Scan(&p.ID, &p.Codigo, &p.Descricao, &p.Preco, &p.Saldo)
	if err != nil {
		http.Error(w, "Produto não encontrado", http.StatusNotFound)
		return
	}

	if p.Saldo < desconto.Quantidade {
		http.Error(w, "Saldo insuficiente", http.StatusBadRequest)
		return
	}

	novoSaldo := p.Saldo - desconto.Quantidade
	_, err = db.Exec(
		"UPDATE produtos SET saldo = ? WHERE id = ?",
		novoSaldo, p.ID,
	)
	if err != nil {
		http.Error(w, "Erro ao atualizar saldo", http.StatusInternalServerError)
		return
	}

	p.Saldo = novoSaldo
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p)
}

func devolverSaldo(w http.ResponseWriter, r *http.Request) {
	var desconto struct {
		ProdutoID  int `json:"produto_id"`
		Quantidade int `json:"quantidade"`
	}

	err := json.NewDecoder(r.Body).Decode(&desconto)
	if err != nil {
		http.Error(w, "Erro ao processar devolução", http.StatusBadRequest)
		return
	}

	_, err = db.Exec(
		"UPDATE produtos SET saldo = saldo + ? WHERE id = ?",
		desconto.Quantidade, desconto.ProdutoID,
	)
	if err != nil {
		http.Error(w, "Erro ao devolver saldo", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func habilitarCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func excluirProduto(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "ID inválido", http.StatusBadRequest)
		return
	}
	db.Exec("DELETE FROM produtos WHERE id = ?", id)
	w.WriteHeader(http.StatusNoContent)
}

func buscarProdutoPorID(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/produtos/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "ID inválido", http.StatusBadRequest)
		return
	}

	var p Produto
	err = db.QueryRow("SELECT id, codigo, descricao, preco, saldo FROM produtos WHERE id = ?", id).
		Scan(&p.ID, &p.Codigo, &p.Descricao, &p.Preco, &p.Saldo)

	if err != nil {
		http.Error(w, "Produto não encontrado", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p)
}

func main() {
	iniciarBanco()

	http.HandleFunc("/produtos", func(w http.ResponseWriter, r *http.Request) {
		habilitarCORS(w)
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		switch r.Method {
		case "GET":
			listarProdutos(w, r)
		case "POST":
			criarProduto(w, r)
		case "DELETE":
			excluirProduto(w, r)
		default:
			http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		}
	})

	http.HandleFunc("/produtos/", func(w http.ResponseWriter, r *http.Request) {
		habilitarCORS(w)

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method == "GET" {
			buscarProdutoPorID(w, r)
		} else {
			http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		}
	})

	http.HandleFunc("/produtos/descontar", func(w http.ResponseWriter, r *http.Request) {
		habilitarCORS(w)
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method == "POST" {
			descontarSaldo(w, r)
		} else {
			http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		}
	})

	http.HandleFunc("/produtos/devolver", func(w http.ResponseWriter, r *http.Request) {
		habilitarCORS(w)
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == "POST" {
			devolverSaldo(w, r)
		} else {
			http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		}
	})

	log.Println("Estoque service rodando na porta 8081...")
	log.Fatal(http.ListenAndServe(":8081", nil))
}
