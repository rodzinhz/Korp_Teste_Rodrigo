import { Component, inject, OnInit, ChangeDetectorRef } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { EstoqueService } from '../../services/estoque';
import { FaturamentoService } from '../../services/faturamento';

interface Produto {
  id: number;
  codigo: string;
  descricao: string;
  categoria?: string;
  preco: number;
  saldo: number;
}

interface ItemTemporario {
  produto_id: number;
  descricao: string;
  qtd: number;
  preco: number;
  total: number;
}

interface ItemDetalhe {
  produto_id: number;
  descricao: string;
  qtd: number;
  preco_unitario: number;
  total: number;
}

interface Nota {
  numero: number;
  status: 'Aberta' | 'Fechada';
  cliente: string;
  valorTotal: number;
  data: string;
  detalhes: ItemDetalhe[];
}

@Component({
  selector: 'app-notas',
  standalone: true,
  imports: [CommonModule, FormsModule],
  templateUrl: './notas.html',
  styleUrl: './notas.css'
})
export class Notas implements OnInit {

  private estoqueService = inject(EstoqueService);
  private faturamentoService = inject(FaturamentoService);
  private cdr = inject(ChangeDetectorRef);

  listaProdutos: Produto[] = [];
  notasEmitidas: Nota[] = [];
  itensDaNota: ItemTemporario[] = [];

  produtoSelecionado: Produto | null = null;
  quantidade: number = 1;
  cliente: string = '';
  notaImprimindo: number | null = null;

  carregando = false;
  // Mensagem de erro exibida em banner na tela — usada especialmente para
  // avisar quando um dos microsserviços está fora do ar (requisito de
  // tratamento de falha), em vez de um alert() que passa despercebido.
  mensagemErro: string | null = null;

  ngOnInit() {
    this.carregarProdutos();
  }

  private extrairMensagemErro(err: any, padrao: string): string {
    if (err?.status === 0) {
      return 'Não foi possível conectar a um dos serviços. Verifique se o backend está rodando.';
    }
    return err?.error?.erro || padrao;
  }

  carregarProdutos() {
    this.estoqueService.getProdutos().subscribe({
      next: (produtos: Produto[]) => {
        this.listaProdutos = produtos;
        this.carregarNotas();
        this.cdr.detectChanges();
      },
      error: (err) => {
        this.mensagemErro = this.extrairMensagemErro(err, 'Erro ao carregar produtos.');
        this.cdr.detectChanges();
      }
    });
  }

  carregarNotas() {
    this.faturamentoService.getNotas().subscribe({
      next: (notas: Nota[]) => {
        this.notasEmitidas = notas ?? [];
        this.cdr.detectChanges();
      },
      error: (err) => {
        this.mensagemErro = this.extrairMensagemErro(err, 'Erro ao carregar notas.');
        this.cdr.detectChanges();
      }
    });
  }

  adicionarItem() {
    const produto = this.produtoSelecionado;
    if (!produto || this.quantidade <= 0) return;

    if (this.quantidade > produto.saldo) {
      this.mensagemErro = `Estoque insuficiente! Você só tem ${produto.saldo} unidade(s) de "${produto.descricao}".`;
      return;
    }

    this.itensDaNota.push({
      produto_id: produto.id,
      descricao: produto.descricao,
      qtd: this.quantidade,
      preco: produto.preco,
      total: produto.preco * this.quantidade
    });

    this.produtoSelecionado = null;
    this.quantidade = 1;
    this.mensagemErro = null;
  }

  removerItem(index: number) {
    this.itensDaNota.splice(index, 1);
  }

  emitirNotaFinal() {
    if (this.itensDaNota.length === 0 || !this.cliente) {
      this.mensagemErro = 'Adicione itens e informe o nome do cliente.';
      return;
    }

    const valorTotal = this.itensDaNota.reduce((acc, item) => acc + item.total, 0);

    // Contrato alinhado com o backend: cada item leva produto_id,
    // descricao, quantidade e preco (preço unitário no momento da venda).
    const nota = {
      cliente: this.cliente,
      valorTotal,
      itens: this.itensDaNota.map(i => ({
        produto_id: i.produto_id,
        descricao: i.descricao,
        quantidade: i.qtd,
        preco: i.preco
      }))
    };

    this.carregando = true;
    this.faturamentoService.criarNota(nota).subscribe({
      next: () => {
        this.carregando = false;
        this.itensDaNota = [];
        this.cliente = '';
        this.mensagemErro = null;
        this.carregarNotas();
        this.cdr.detectChanges();
      },
      error: (err) => {
        this.carregando = false;
        this.mensagemErro = this.extrairMensagemErro(err, 'Erro ao criar nota.');
        this.cdr.detectChanges();
      }
    });
  }

  imprimirNota(nota: Nota) {
    if (nota.status === 'Fechada') return;

    this.notaImprimindo = nota.numero;
    this.mensagemErro = null;

    this.faturamentoService.imprimirNota(nota.numero).subscribe({
      next: () => {
        this.notaImprimindo = null;
        this.carregarNotas();
        this.carregarProdutos();
        this.cdr.detectChanges();
      },
      error: (err) => {
        this.notaImprimindo = null;
        // Cenário de falha de microsserviço: o backend já reverteu qualquer
        // desconto parcial de estoque, então a tela só precisa informar.
        this.mensagemErro = this.extrairMensagemErro(
          err,
          'Erro ao imprimir nota — serviço de estoque indisponível. Nenhuma alteração foi aplicada.'
        );
        this.cdr.detectChanges();
      }
    });
  }

  excluirNota(nota: Nota) {
    if (!confirm('Tem certeza que deseja excluir esta nota?')) return;

    this.faturamentoService.excluirNota(nota.numero).subscribe({
      next: () => {
        this.carregarNotas();
        this.cdr.detectChanges();
      },
      error: (err) => {
        this.mensagemErro = this.extrairMensagemErro(err, 'Erro ao excluir nota.');
        this.cdr.detectChanges();
      }
    });
  }
}