import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { timeout } from 'rxjs'; // 👈 Importação adicionada

@Injectable({
  providedIn: 'root'
})
export class FaturamentoService {

  private http = inject(HttpClient);
  private api = 'http://localhost:8082';

  getNotas() {
    return this.http.get<any[]>(`${this.api}/notas`).pipe(timeout(5000));
  }

  // Gera uma chave única por operação. Se a requisição falhar por timeout
  // e o Angular tentar de novo com a MESMA chave, o backend reconhece que
  // já processou e devolve o mesmo resultado, em vez de duplicar a nota
  // ou descontar o estoque duas vezes.
  private gerarChaveIdempotencia(): string {
    return crypto.randomUUID();
  }

  criarNota(nota: any) {
    const headers = { 'Idempotency-Key': this.gerarChaveIdempotencia() };
    return this.http.post(`${this.api}/notas`, nota, { headers }).pipe(timeout(5000));
  }

  imprimirNota(id: number) {
    const headers = { 'Idempotency-Key': this.gerarChaveIdempotencia() };
    return this.http.put(`${this.api}/notas?id=${id}`, {}, { headers }).pipe(timeout(5000));
  }

  excluirNota(id: number) {
    return this.http.delete(`${this.api}/notas?id=${id}`).pipe(timeout(5000));
  }
}