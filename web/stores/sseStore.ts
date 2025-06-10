import { defineStore } from 'pinia';
import { v4 as uuidv4 } from 'uuid';
import { useTicketStore, type Ticket } from './ticketStore';

// --- Interfaces ---
interface SseStatus {
    text: string;
    color: string;
}

interface PromotionEvent {
  id: string;
  nomePromocao: string;
  descricao: string;
}

interface TicketGeneratedEvent {
    ticketId: string;
    reservationId: string;
    customer: string;
    cruiseId: number;
    issuedAt: string;
}

interface PaymentStatusEvent {
    reservationId: string;
    status: 'aprovada' | 'recusada';
}

const API_BASE_URL = 'http://localhost:8080';

export const useSseStore = defineStore('sse', {
  state: () => ({
    clientId: null as string | null,
    status: { text: 'Desconectado', color: 'bg-slate-200 text-slate-700' } as SseStatus,
    lastPromotion: null as PromotionEvent | null,
    paymentEvents: {} as Record<string, PaymentStatusEvent>,
    ticketEvents: {} as Record<string, TicketGeneratedEvent>,
    isInterestedInPromo: false, // Novo estado para o interesse
  }),

  actions: {
    initialize() {
      if (typeof window === 'undefined') return;

      // Carregar preferência do localStorage
      this.isInterestedInPromo = localStorage.getItem('promoInterest') === 'true';

      this.connect();
    },

    connect() {
      // Evita múltiplas conexões
      if (this.clientId || typeof window === 'undefined') {
        return;
      }

      this.clientId = localStorage.getItem('sseClientId') || uuidv4();
      localStorage.setItem('sseClientId', this.clientId);

      const eventSource = new EventSource(`${API_BASE_URL}/notifications/${this.clientId}`);

      eventSource.onopen = () => {
        this.status = { text: 'Conectado', color: 'bg-green-200 text-green-800' };
        // Sincroniza o interesse com o backend ao conectar
        this.updateInterestOnServer(this.isInterestedInPromo);
      };

      eventSource.onerror = () => {
        this.status = { text: 'Erro de Conexão', color: 'bg-red-200 text-red-800' };
      };

      // Listeners...
      eventSource.addEventListener('promocao', (event: MessageEvent) => {
        if (this.isInterestedInPromo) {
            this.lastPromotion = JSON.parse(event.data);
        }
      });

      eventSource.addEventListener('pagamento_aprovado', (event: MessageEvent) => {
        const data: PaymentStatusEvent = JSON.parse(event.data);
        this.paymentEvents[data.reservationId] = { ...data, status: 'aprovada' };
      });

      eventSource.addEventListener('pagamento_recusado', (event: MessageEvent) => {
        const data: PaymentStatusEvent = JSON.parse(event.data);
        this.paymentEvents[data.reservationId] = { ...data, status: 'recusada' };
      });

      eventSource.addEventListener('bilhete_gerado', (event: MessageEvent) => {
        const ticketData: TicketGeneratedEvent = JSON.parse(event.data);
        this.ticketEvents[ticketData.reservationId] = ticketData;

        const ticketStore = useTicketStore();
        const newTicket: Ticket = {
            ...ticketData,
            cruiseName: "Nome do Cruzeiro"
        };
        ticketStore.addTicket(newTicket);
      });
    },

    // Nova ação para atualizar o interesse
    async togglePromotionInterest() {
        const newInterestState = !this.isInterestedInPromo;
        await this.updateInterestOnServer(newInterestState);
        this.isInterestedInPromo = newInterestState;
        localStorage.setItem('promoInterest', String(newInterestState));
    },

    async updateInterestOnServer(interested: boolean) {
        if (!this.clientId) return;
        await $fetch(`${API_BASE_URL}/notifications/interest`, {
            method: 'POST',
            body: {
                clientId: this.clientId,
                interested,
            }
        });
    },

    clearPromotion() {
        this.lastPromotion = null;
    }
  },
});
