<template>
  <div class="max-w-2xl mx-auto bg-white p-8 rounded-xl shadow-xl">
    <div v-if="cruzeiroPending" class="text-center text-slate-600 py-6">A carregar informações da reserva...</div>
    <div v-else-if="cruzeiroError || !cruzeiroData" class="text-center text-red-500 bg-red-100 p-4 rounded-lg">
      Erro ao carregar dados do cruzeiro para reserva. Por favor, tente novamente.
      <NuxtLink to="/" class="block mt-2 text-sky-600 hover:underline">Voltar à lista</NuxtLink>
    </div>
    <div v-else>
      <h1 class="text-3xl font-bold text-sky-800 mb-2">Reservar Cruzeiro</h1>
      <h2 class="text-xl text-sky-600 font-semibold mb-6">{{ cruzeiroData.nome }}</h2>

      <!-- Formulário de Reserva -->
      <form @submit.prevent="handleReservation" v-if="!reservationId">
        <div class="mb-4">
          <label for="customerName" class="block text-sm font-medium text-slate-700 mb-1">Nome Completo:</label>
          <input type="text" id="customerName" v-model="reservationForm.customer" required
                 class="w-full px-4 py-2 border border-slate-300 rounded-lg shadow-sm focus:ring-sky-500 focus:border-sky-500 transition-colors" />
        </div>
        <div class="grid grid-cols-2 gap-4 mb-4">
            <div>
                <label for="passengers" class="block text-sm font-medium text-slate-700 mb-1">Passageiros:</label>
                <select id="passengers" v-model.number="reservationForm.passengers" required
                        class="w-full px-4 py-2 border border-slate-300 rounded-lg shadow-sm focus:ring-sky-500 focus:border-sky-500 transition-colors">
                    <option v-for="n in 4" :key="n" :value="n">{{ n }}</option>
                </select>
            </div>
            <div>
                <label for="cabins" class="block text-sm font-medium text-slate-700 mb-1">Nº de Cabines:</label>
                <select id="cabins" v-model.number="reservationForm.numeroCabines" required
                        class="w-full px-4 py-2 border border-slate-300 rounded-lg shadow-sm focus:ring-sky-500 focus:border-sky-500 transition-colors">
                    <option v-for="n in 2" :key="n" :value="n">{{ n }}</option>
                </select>
            </div>
        </div>

        <div class="mb-6 p-4 bg-sky-50 rounded-lg text-center">
          <p class="text-lg font-semibold text-slate-700">Valor Total da Reserva:</p>
          <p class="text-3xl font-bold text-emerald-600">R$ {{ totalPrice.toFixed(2).replace('.', ',') }}</p>
        </div>

        <button type="submit" :disabled="reservationPending"
                class="w-full bg-emerald-500 hover:bg-emerald-600 text-white font-bold py-3 px-6 rounded-lg text-lg transition-colors shadow-md hover:shadow-lg disabled:opacity-50 disabled:cursor-not-allowed">
          <span v-if="reservationPending">A processar...</span>
          <span v-else>Confirmar e Ir para Pagamento</span>
        </button>
         <p v-if="reservationError" class="text-red-600 text-center mt-4">{{ reservationError }}</p>
      </form>

      <!-- Área de Status da Reserva Pós-Criação -->
      <div v-else class="mt-6 text-center">
        <h2 class="text-2xl font-bold text-green-600">Reserva Criada!</h2>
        <p class="text-slate-700 mt-2">A sua reserva <span class="font-semibold">{{ reservationId }}</span> foi criada com sucesso.</p>
<!--        <p class="mt-4">O próximo passo é realizar o pagamento.</p>-->

<!--        <a :href="paymentLink" target="_blank"-->
<!--           class="inline-block mt-4 mb-6 bg-sky-600 hover:bg-sky-700 text-white font-bold py-3 px-8 rounded-lg text-lg transition-transform hover:scale-105">-->
<!--           Clique Aqui para Pagar-->
<!--        </a>-->

        <div class="space-y-4 p-4 border border-slate-200 rounded-lg">
            <div class="flex items-center justify-center gap-3">
                <span class="font-semibold">Status do Pagamento:</span>
                <span :class="paymentStatus.color" class="px-3 py-1 text-sm font-medium rounded-full">{{ paymentStatus.text }}</span>
            </div>
             <div v-if="ticketStatus" class="flex items-center justify-center gap-3">
                <span class="font-semibold">Status do Bilhete:</span>
                <span :class="ticketStatus.color" class="px-3 py-1 text-sm font-medium rounded-full">{{ ticketStatus.text }}</span>
            </div>
        </div>

        <div v-if="finalMessage" class="mt-6 p-4 rounded-lg" :class="isSuccess ? 'bg-green-100 text-green-800' : 'bg-red-100 text-red-800'">
            <p class="font-bold">{{ finalMessage }}</p>
            <NuxtLink v-if="isSuccess" to="/meus-bilhetes" class="mt-2 inline-block text-sm font-semibold text-green-800 hover:underline">
                Ver os meus bilhetes &rarr;
            </NuxtLink>
        </div>

        <button v-if="!isFinalState" @click="handleCancelReservation" :disabled="cancellationPending"
                class="mt-6 w-full bg-red-500 hover:bg-red-600 text-white font-semibold py-2 px-4 rounded-lg transition-colors disabled:opacity-50">
            <span v-if="cancellationPending">A cancelar...</span>
            <span v-else>Cancelar Reserva</span>
        </button>
        <p v-if="cancellationError" class="text-red-600 text-center mt-2">{{ cancellationError }}</p>
      </div>

    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch } from 'vue';
import { useSseStore } from '~/stores/sseStore';

// --- Interfaces ---
interface CruzeiroParaReserva {
  id: number;
  nome: string;
  valorCabine: number;
  dataEmbarque: string;
}
// CORRIGIDO: Adicionado clientId ao payload da reserva
interface ReservationPayload {
  clientId: string;
  cruiseId: number;
  customer: string;
  passengers: number;
  numeroCabines: number;
  dataEmbarque: string;
  valorTotal: number;
}
interface PaymentLinkResponse {
    reservationId: string;
    paymentLink: string;
}
interface Status {
    text: string;
    color: string;
}

// --- Configuração ---
const route = useRoute();
const sseStore = useSseStore();
const cruzeiroId = computed(() => parseInt(route.params.id as string));
const API_BASE_URL = 'http://localhost:8080';

// --- State ---
const reservationForm = reactive({ customer: '', passengers: 1, numeroCabines: 1 });
const totalPrice = computed(() => (cruzeiroData.value?.valorCabine || 0) * reservationForm.numeroCabines);
const reservationPending = ref(false);
const reservationError = ref<string | null>(null);
const reservationId = ref<string | null>(null);
const paymentLink = ref<string>('');

// --- Status reativos baseados na store SSE ---
const paymentStatus = ref<Status>({ text: 'A aguardar', color: 'bg-amber-200 text-amber-800' });
const ticketStatus = ref<Status | null>(null);
const finalMessage = ref('');
const isSuccess = ref(false);
const isFinalState = ref(false);
const cancellationPending = ref(false);
const cancellationError = ref<string | null>(null);

// --- Data Fetching ---
const { data: cruzeiros, pending: cruzeiroPending, error: cruzeiroError } = await useAsyncData<CruzeiroParaReserva[]>(
    `itineraries-list-reserve-${cruzeiroId.value}`,
    () => $fetch(`${API_BASE_URL}/itineraries`)
);
const cruzeiroData = computed(() => cruzeiros.value?.find(c => c.id === cruzeiroId.value));

// --- Lógica de Reserva ---
async function handleReservation() {
  // CORRIGIDO: Adicionada verificação para garantir que o clientId existe antes de enviar.
  if (!cruzeiroData.value || !sseStore.clientId) {
      reservationError.value = "Não foi possível iniciar a reserva. Verifique a sua conexão com o servidor.";
      return;
  }
  reservationPending.value = true;
  reservationError.value = null;

  const payload: ReservationPayload = {
    // CORRIGIDO: clientId agora é incluído no payload.
    clientId: sseStore.clientId,
    cruiseId: cruzeiroData.value.id,
    customer: reservationForm.customer,
    passengers: reservationForm.passengers,
    numeroCabines: reservationForm.numeroCabines,
    valorTotal: totalPrice.value,
    dataEmbarque: cruzeiroData.value.dataEmbarque,
  };

  try {
    const response = await $fetch<PaymentLinkResponse>(`${API_BASE_URL}/reservations`, {
      method: 'POST',
      body: payload,
    });
    reservationId.value = response.reservationId;
    paymentLink.value = response.paymentLink;
  } catch (err: any) {
    reservationError.value = err.data?.error || 'Falha ao criar a reserva.';
  } finally {
    reservationPending.value = false;
  }
}

// --- Lógica de Cancelamento ---
async function handleCancelReservation() {
    if (!reservationId.value) return;
    cancellationPending.value = true;
    cancellationError.value = null;
    try {
        // CORRIGIDO: Alterado para usar o método DELETE e o ID na URL.
        await $fetch(`${API_BASE_URL}/reservations/${reservationId.value}`, {
            method: 'DELETE',
        });
        finalMessage.value = "A sua reserva foi cancelada com sucesso.";
        isFinalState.value = true;
        isSuccess.value = false;
    } catch(err: any) {
        cancellationError.value = err.data?.error || 'Falha ao cancelar a reserva.';
    } finally {
        cancellationPending.value = false;
    }
}

// --- Watchers para reagir a eventos da store SSE ---
watch(() => sseStore.paymentEvents[reservationId.value!], (event) => {
    if (!event) return;
    if (event.status === 'aprovada') {
        paymentStatus.value = { text: 'Aprovado', color: 'bg-green-200 text-green-800' };
        ticketStatus.value = { text: 'A processar...', color: 'bg-sky-200 text-sky-800' };
    } else {
        paymentStatus.value = { text: 'Recusado', color: 'bg-red-200 text-red-800' };
        finalMessage.value = 'Pagamento recusado. A sua reserva foi cancelada automaticamente.';
        isFinalState.value = true;
    }
});

watch(() => sseStore.ticketEvents[reservationId.value!], (event) => {
    if (!event) return;
    ticketStatus.value = { text: 'Emitido', color: 'bg-green-200 text-green-800' };
    finalMessage.value = `Viagem confirmada! O seu bilhete é ${event.ticketId}.`;
    isFinalState.value = true;
    isSuccess.value = true;
});

useHead({
  title: `Reservar ${cruzeiroData.value?.nome || 'Cruzeiro'}`,
});
</script>
