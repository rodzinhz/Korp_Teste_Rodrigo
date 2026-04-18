import { Component, inject, OnInit, NgZone, ChangeDetectorRef} from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { EstoqueService } from '../../services/estoque';

@Component({
  selector: 'app-produtos',
  standalone: true,
  imports: [CommonModule, FormsModule],
  templateUrl: './produtos.html',
  styleUrl: './produtos.css',
})
export class Produtos implements OnInit {

  private estoqueService = inject(EstoqueService);
  private zone = inject(NgZone);
  private cdr = inject(ChangeDetectorRef);

  listaProdutos: any[] = [];
  nomeNovoProduto: string = '';
  codigoNovoProduto: string = '';
  precoNovoProduto: number | null = null;
  saldoNovoProduto: number | null = null;

  ngOnInit() {
      this.carregarProdutos();
  }
    
      

  carregarProdutos() {
    this.estoqueService.getProdutos().subscribe({
      next: (produtos: any[]) => {
        console.log('Produtos carregados:', produtos); 
       
          this.listaProdutos = [...produtos];
          this.cdr.detectChanges();
      },
      error: (err: any) => console.error('Erro ao carregar produtos:', err)
    });
  }

 adicionar() {
  if (!this.codigoNovoProduto || !this.nomeNovoProduto || !this.precoNovoProduto) {
    alert("Preencha todos os campos!");
    return;
  }

  if (!this.saldoNovoProduto || this.saldoNovoProduto <= 0) {
    alert("Quantidade deve ser maior que zero!");
    return;
  }

  const novo = {
    codigo: this.codigoNovoProduto,
    descricao: this.nomeNovoProduto,
    preco: this.precoNovoProduto,
    saldo: this.saldoNovoProduto
  };

  this.estoqueService.adicionarProduto(novo).subscribe({
    next: () => {
      this.carregarProdutos();
      this.codigoNovoProduto = '';
      this.nomeNovoProduto = '';
      this.precoNovoProduto = null;
      this.saldoNovoProduto = null;
    },
    error: (err: any) => {
      alert("Erro ao adicionar produto!");
      console.error(err);
    }
  });
}

  excluirProduto(id: number) {
    if (!confirm("Tem certeza que deseja excluir este produto?")) return;

    this.estoqueService.excluirProduto(id).subscribe({
      next: () => {
        alert('Produto excluído com sucesso!');
        this.carregarProdutos(); 
      },
      error: (err: any) => {
        alert('Erro ao excluir produto!');
        console.error(err);
      }
    });
  }
}
