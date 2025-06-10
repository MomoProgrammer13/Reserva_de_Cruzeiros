<template>
  <div>
    <h1 class="text-4xl font-bold text-sky-800 mb-8 text-center">Meus Bilhetes</h1>

    <div v-if="sortedTickets.length === 0" class="text-center text-slate-600 bg-white p-10 rounded-xl shadow">
      <p class="text-lg">Você ainda não comprou nenhum bilhete.</p>
      <NuxtLink to="/" class="mt-4 inline-block bg-sky-600 hover:bg-sky-700 text-white font-semibold py-2 px-4 rounded-lg transition-colors">
        Ver Cruzeiros Disponíveis
      </NuxtLink>
    </div>

    <div v-else class="grid grid-cols-1 lg:grid-cols-2 gap-6">
      <TicketCard
        v-for="ticket in sortedTickets"
        :key="ticket.ticketId"
        :ticket="ticket"
        :is-cancelling="cancellingState[ticket.reservationId]?.pending"
        @cancel="handleCancelReservation(ticket.reservationId)"
      />
    </div>

    <!-- Notificação de Erro -->
    <div v-if="cancellationError" class="fixed bottom-10 right-10 bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded-lg shadow-lg z-50">
      <strong class="font-bold">Erro!</strong>
      <span class="block sm:inline"> {{ cancellationError }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import { useTicketStore } from '~/stores/ticketStore';
import TicketCard from '~/components/TicketCard.vue';

const ticketStore = useTicketStore();
const sortedTickets = computed(() => ticketStore.sortedTickets);
const API_BASE_URL = 'http://localhost:8080';

// Estado para controlar o cancelamento de cada bilhete individualmente
const cancellingState = ref<Record<string, { pending: boolean }>>({});
const cancellationError = ref<string | null>(null);

async function handleCancelReservation(reservationId: string) {
    if (!window.confirm('Tem a certeza de que deseja cancelar esta reserva? Esta ação não pode ser desfeita.')) {
        return;
    }

    cancellingState.value[reservationId] = { pending: true };
    cancellationError.value = null;

    try {
        // Chamada atualizada para usar DELETE e o ID na URL
        await $fetch(`${API_BASE_URL}/reservations/${reservationId}`, {
            method: 'DELETE',
        });

        // Em caso de sucesso, remove o bilhete da store
        ticketStore.removeTicketByReservationId(reservationId);

    } catch (err: any) {
        cancellationError.value = err.data?.error || 'Não foi possível cancelar a reserva.';
        setTimeout(() => { cancellationError.value = null; }, 5000);
    } finally {
        delete cancellingState.value[reservationId];
    }
}

useHead({
  title: 'Os Meus Bilhetes - Reserva de Cruzeiros',
});
</script>
