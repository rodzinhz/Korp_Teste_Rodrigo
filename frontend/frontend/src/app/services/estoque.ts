import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';

@Injectable({
  providedIn: 'root'
})
export class EstoqueService {

  private http = inject(HttpClient);
  private api = 'http://localhost:8081';

  getProdutos() {
    return this.http.get<any[]>(`${this.api}/produtos`);
  }

  adicionarProduto(produto: any) {
    return this.http.post(`${this.api}/produtos`, produto);
  }

  descontarSaldo(produto_id: number, quantidade: number) {
    return this.http.post(`${this.api}/produtos/descontar`, { produto_id, quantidade });
  }

  excluirProduto(id: number) {
    return this.http.delete(`${this.api}/produtos?id=${id}`);
  }
}