import { defineStore } from 'pinia';

// Interface para definir a estrutura de um bilhete
export interface Ticket {
  ticketId: string;
  reservationId: string;
  customer: string;
  cruiseId: number;
  cruiseName: string; // Adicionado para exibir o nome do cruzeiro facilmente
  issuedAt: string;
}

const TICKET_STORAGE_KEY = 'cruise_tickets';

export const useTicketStore = defineStore('tickets', {
  state: () => ({
    tickets: [] as Ticket[],
  }),

  getters: {
    // Getter para obter tickets em ordem, do mais recente para o mais antigo
    sortedTickets: (state): Ticket[] => {
      return [...state.tickets].sort((a, b) => new Date(b.issuedAt).getTime() - new Date(a.issuedAt).getTime());
    },
  },

  actions: {
    /**
     * Carrega os bilhetes do localStorage para o estado da store.
     * Deve ser chamado quando o aplicativo é inicializado (ex: no layout principal).
     */
    loadTicketsFromStorage() {
      if (typeof window !== 'undefined') {
        const storedTickets = localStorage.getItem(TICKET_STORAGE_KEY);
        if (storedTickets) {
          this.tickets = JSON.parse(storedTickets);
        }
      }
    },

    /**
     * Adiciona um novo bilhete ao estado e persiste no localStorage.
     * @param newTicket O objeto do bilhete a ser adicionado.
     */
    addTicket(newTicket: Ticket) {
      // Evita adicionar duplicatas
      const exists = this.tickets.some(t => t.ticketId === newTicket.ticketId);
      if (!exists) {
        this.tickets.push(newTicket);
        this.persistTickets();
      }
    },

    /**
     * Remove um bilhete do estado e do localStorage pelo ID da sua reserva.
     * @param reservationId O ID da reserva do bilhete a ser removido.
     */
    removeTicketByReservationId(reservationId: string) {
      const initialLength = this.tickets.length;
      this.tickets = this.tickets.filter(t => t.reservationId !== reservationId);
      if (this.tickets.length < initialLength) {
        this.persistTickets();
      }
    },

    /**
     * Salva a lista atual de bilhetes no localStorage.
     */
    persistTickets() {
      if (typeof window !== 'undefined') {
        localStorage.setItem(TICKET_STORAGE_KEY, JSON.stringify(this.tickets));
      }
    },
  },
});
