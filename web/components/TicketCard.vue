<template>
  <div class="bg-white rounded-xl shadow-lg overflow-hidden border-l-4 border-sky-500 flex flex-col">
    <div class="p-6 flex-grow">
      <div class="flex justify-between items-start">
        <div>
          <h3 class="text-2xl font-bold text-sky-800">{{ ticket.cruiseName }}</h3>
          <p class="text-sm text-slate-500">Cliente: {{ ticket.customer }}</p>
        </div>
        <div class="text-right">
            <span class="inline-block bg-green-100 text-green-800 text-xs font-semibold px-2.5 py-1 rounded-full">EMITIDO</span>
        </div>
      </div>

      <div class="mt-4 pt-4 border-t border-slate-200">
        <ul class="space-y-2 text-sm text-slate-600">
          <li class="flex justify-between">
            <span class="font-semibold">ID do Bilhete:</span>
            <span>{{ ticket.ticketId }}</span>
          </li>
          <li class="flex justify-between">
            <span class="font-semibold">ID da Reserva:</span>
            <span>{{ ticket.reservationId }}</span>
          </li>
          <li class="flex justify-between">
            <span class="font-semibold">Data de Emissão:</span>
            <span>{{ formatDate(ticket.issuedAt) }}</span>
          </li>
        </ul>
      </div>
    </div>
    <div class="bg-slate-50 p-4 mt-auto flex justify-end">
        <button
            @click="$emit('cancel')"
            :disabled="isCancelling"
            class="bg-red-500 hover:bg-red-600 text-white font-semibold py-1 px-3 text-sm rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed">
            <span v-if="isCancelling">A cancelar...</span>
            <span v-else>Cancelar Reserva</span>
        </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { Ticket } from '~/stores/ticketStore';

defineProps<{
  ticket: Ticket;
  isCancelling?: boolean;
}>();

defineEmits(['cancel']);

function formatDate(dateString: string) {
  if (!dateString) return 'N/A';
  return new Date(dateString).toLocaleString('pt-PT', {
    day: '2-digit',
    month: '2-digit',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}
</script>
