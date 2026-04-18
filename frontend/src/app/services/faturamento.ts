import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';

@Injectable({
  providedIn: 'root'
})
export class FaturamentoService {

  private http = inject(HttpClient);
  private api = 'http://localhost:8082';

  getNotas() {
    return this.http.get<any[]>(`${this.api}/notas`);
  }

  criarNota(nota: any) {
    return this.http.post(`${this.api}/notas`, nota);
  }

  imprimirNota(id: number) {
    return this.http.put(`${this.api}/notas?id=${id}`, {});
  }

  excluirNota(id: number) {
    return this.http.delete(`${this.api}/notas?id=${id}`);
  }
}