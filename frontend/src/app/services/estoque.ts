import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { timeout } from 'rxjs'; 

@Injectable({
  providedIn: 'root'
})
export class EstoqueService {

  private http = inject(HttpClient);
  private api = 'http://localhost:8081';

  getProdutos() {
    return this.http.get<any[]>(`${this.api}/produtos`).pipe(timeout(5000)); 
  }

  adicionarProduto(produto: any) {
    return this.http.post(`${this.api}/produtos`, produto).pipe(timeout(5000));
  }

  descontarSaldo(produto_id: number, quantidade: number) {
    return this.http.post(`${this.api}/produtos/descontar`, { produto_id, quantidade }).pipe(timeout(5000));
  }

  excluirProduto(id: number) {
    return this.http.delete(`${this.api}/produtos?id=${id}`).pipe(timeout(5000));
  }

  sugerirDescricao(texto: string) { 
    return this.http.post<{ descricao: string; categoria: string }>(
      `${this.api}/produtos/sugerir-descricao`,
      { texto }
    ).pipe(timeout(5000));
  }
}