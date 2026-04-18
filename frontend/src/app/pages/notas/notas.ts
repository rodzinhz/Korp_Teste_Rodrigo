import { Component, inject, OnInit, ChangeDetectorRef } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { EstoqueService } from '../../services/estoque';
import { FaturamentoService } from '../../services/faturamento';

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

  listaProdutos: any[] = [];
  notasEmitidas: any[] = [];
  itensDaNota: any[] = [];

  produtoSelecionado: any = null;
  quantidade: number = 1;
  cliente: string = '';
  notaImprimindo: number | null = null;

  ngOnInit() {
    this.carregarProdutos();
    this.cdr.detectChanges();
  }

  carregarProdutos() {
  this.estoqueService.getProdutos().subscribe({
    next: (produtos: any[]) => {
      this.listaProdutos = produtos.map(p => ({
        ...p,
        nome: p.descricao  
      }));
      console.log('primeiro produto:', this.listaProdutos[0]);
      this.carregarNotas(); 
    },
    error: (err: any) => console.error('Erro ao carregar produtos:', err)
  });
}

carregarNotas() {
  this.faturamentoService.getNotas().subscribe({
    next: (notas: any[]) => {
      this.notasEmitidas = (notas ?? []).map((n: any) => ({
        ...n,
        numero: n.numero || n.id,
        cliente: n.cliente || '',
        valorTotal: n.valorTotal || 0,
        data: n.data || '',
        detalhes: (n.detalhes || []).map((item: any) => {
          console.log('Item da nota:', item);
          const produto = this.listaProdutos.find((p: any) => p.id === item.produto_id);
          return {
            descricao: item.nome || 'Produto não encontrado', 
            qtd: item.qtd,
            total: item.total, 
            preco_unitario: item.preco_unitario
          };
        })
      }));
      
      
      this.cdr.detectChanges(); 
    },
    error: (err: any) => console.error('Erro ao carregar notas:', err)
  });
}

  adicionarItem() {
  const produto = this.produtoSelecionado;

  if (produto && this.quantidade > 0) {
    if (this.quantidade > produto.saldo) {
      alert(`Estoque insuficiente! Você só tem ${produto.saldo} unidade(s).`);
      return;
    }

    this.itensDaNota.push({
      produto_id: produto.id,
      descricao: produto.descricao,
      qtd: this.quantidade,
      total: produto.preco * this.quantidade
    });

    this.produtoSelecionado = null;
    this.quantidade = 1;
    this.cdr.detectChanges(); 
  }
}

  emitirNotaFinal() {
    if (this.itensDaNota.length === 0 || !this.cliente) {
      alert("Adicione itens e informe o nome do cliente!");
      return;
    }

    const valorTotal = this.itensDaNota.reduce((acc, item) => {
      return acc + (item.total || 0); 
    }, 0);

    const nota = {
      cliente: this.cliente, 
      valorTotal: valorTotal,
      data: new Date().toLocaleDateString('pt-BR'), 
      itens: this.itensDaNota.map(i => ({
        nome: i.nome,
        produto_id: i.produto_id,
        quantidade: i.qtd,
        preco: i.total / i.qtd
      }))
    };

    this.faturamentoService.criarNota(nota).subscribe({
      next: () => {
        alert('Nota criada com sucesso!');
        this.itensDaNota = [];
        this.cliente = ''; 
        this.carregarNotas();
      },
      error: (err: any) => {
        console.error('Erro ao criar nota:', err);
        alert('Erro ao criar nota!');
      }
    });
  }

  imprimirNota(nota: any) {
  if (nota.status === 'Fechada') {
    alert("Esta nota já está fechada.");
    return;
  }

  this.notaImprimindo = nota.numero

  this.faturamentoService.imprimirNota(nota.numero).subscribe({
    next: () => {
      this.notaImprimindo = null;
      alert('Nota impressa e fechada com sucesso!');
      this.carregarNotas();
      this.carregarProdutos();
      this.cdr.detectChanges();
    },
    error: (err: any) => {
      this.notaImprimindo = null;
      alert('Erro ao imprimir nota — serviço de estoque indisponível!');
      console.error(err);
    }
  });
}

excluirNota(nota: any) { 
  if (confirm("Tem certeza que deseja excluir esta nota?")) {
    this.faturamentoService.excluirNota(nota.numero).subscribe({
      next: () => {
        alert('Nota excluída com sucesso!');
        this.carregarNotas();
      },
      error: (err: any) => {
        alert('Erro ao excluir nota!');
        console.error(err);
      }
    });
  }
}
}