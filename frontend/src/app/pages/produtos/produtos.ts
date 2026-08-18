import { Component, inject, OnInit, ChangeDetectorRef } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { EstoqueService } from '../../services/estoque';

interface Produto {
  id: number;
  codigo: string;
  descricao: string;
  categoria?: string;
  preco: number;
  saldo: number;
}

@Component({
  selector: 'app-produtos',
  standalone: true,
  imports: [CommonModule, FormsModule],
  templateUrl: './produtos.html',
  styleUrl: './produtos.css',
})
export class Produtos implements OnInit {

  private estoqueService = inject(EstoqueService);
  private cdr = inject(ChangeDetectorRef);

  listaProdutos: Produto[] = [];

  codigoNovoProduto: string = '';
  nomeNovoProduto: string = '';
  categoriaNovoProduto: string = '';
  precoNovoProduto: number | null = null;
  saldoNovoProduto: number | null = null;

  sugerindoIA = false;
  mensagemErro: string | null = null;

  ngOnInit() {
    this.carregarProdutos();
  }

  private extrairMensagemErro(err: any, padrao: string): string {
    if (err?.status === 0) {
      return 'Não foi possível conectar ao serviço de estoque. Verifique se o backend está rodando.';
    }
    return err?.error?.erro || padrao;
  }

  carregarProdutos() {
    this.estoqueService.getProdutos().subscribe({
      next: (produtos: Produto[]) => {
        this.listaProdutos = produtos;
        this.cdr.detectChanges();
      },
      error: (err) => {
        this.mensagemErro = this.extrairMensagemErro(err, 'Erro ao carregar produtos.');
        this.cdr.detectChanges();
      }
    });
  }

  // Requisito opcional "Uso de Inteligência Artificial": o usuário digita
  // algo curto no campo Nome (ex: "parafuso sextavado 6mm") e a IA sugere
  // uma descrição comercial completa + categoria, preenchendo os campos.
  sugerirComIA() {
    if (!this.nomeNovoProduto) {
      this.mensagemErro = 'Digite algo no campo Nome para a IA sugerir a descrição.';
      return;
    }

    this.sugerindoIA = true;
    this.estoqueService.sugerirDescricao(this.nomeNovoProduto).subscribe({
      next: (sugestao) => {
        this.sugerindoIA = false;
        if (sugestao.descricao) this.nomeNovoProduto = sugestao.descricao;
        if (sugestao.categoria) this.categoriaNovoProduto = sugestao.categoria;
        this.cdr.detectChanges();
      },
      error: (err) => {
        this.sugerindoIA = false;
        this.mensagemErro = this.extrairMensagemErro(err, 'Não foi possível gerar a sugestão de IA.');
        this.cdr.detectChanges();
      }
    });
  }

  adicionar() {
    if (!this.codigoNovoProduto || !this.nomeNovoProduto || !this.precoNovoProduto) {
      this.mensagemErro = 'Preencha código, nome e preço.';
      return;
    }
    if (!this.saldoNovoProduto || this.saldoNovoProduto <= 0) {
      this.mensagemErro = 'Quantidade deve ser maior que zero.';
      return;
    }

    const novo = {
      codigo: this.codigoNovoProduto,
      descricao: this.nomeNovoProduto,
      categoria: this.categoriaNovoProduto,
      preco: this.precoNovoProduto,
      saldo: this.saldoNovoProduto
    };

    this.estoqueService.adicionarProduto(novo).subscribe({
      next: () => {
        this.carregarProdutos();
        this.codigoNovoProduto = '';
        this.nomeNovoProduto = '';
        this.categoriaNovoProduto = '';
        this.precoNovoProduto = null;
        this.saldoNovoProduto = null;
        this.mensagemErro = null;
        this.cdr.detectChanges();
      },
      error: (err) => {
        this.mensagemErro = this.extrairMensagemErro(err, 'Erro ao adicionar produto.');
        this.cdr.detectChanges();
      }
    });
  }

  excluirProduto(id: number) {
    if (!confirm('Tem certeza que deseja excluir este produto?')) return;

    this.estoqueService.excluirProduto(id).subscribe({
      next: () => {
        this.carregarProdutos();
        this.cdr.detectChanges();
      },
      error: (err) => {
        this.mensagemErro = this.extrairMensagemErro(err, 'Erro ao excluir produto.');
        this.cdr.detectChanges();
      }
    });
  }
}