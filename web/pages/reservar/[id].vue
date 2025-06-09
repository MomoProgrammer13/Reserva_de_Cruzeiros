<template>
  <div class="max-w-2xl mx-auto bg-white p-8 rounded-xl shadow-xl">
    <div v-if="cruzeiroPending" class="text-center text-slate-600 py-6">Carregando informações da reserva...</div>
    <div v-else-if="cruzeiroError || !cruzeiroData" class="text-center text-red-500 bg-red-100 p-4 rounded-lg">
      Erro ao carregar dados do cruzeiro para reserva. Por favor, tente novamente.
      <NuxtLink to="/" class="block mt-2 text-sky-600 hover:underline">Voltar à lista</NuxtLink>
    </div>
    <div v-else>
      <h1 class="text-3xl font-bold text-sky-800 mb-2">Reservar Cruzeiro</h1>
      <h2 class="text-xl text-sky-600 font-semibold mb-6">{{ cruzeiroData.nome }}</h2>

      <form @submit.prevent="handleReservation">
        <div class="mb-6">
          <label for="customerName" class="block text-sm font-medium text-slate-700 mb-1">Nome Completo:</label>
          <input type="text" id="customerName" v-model="reservationForm.customer" required
                 class="w-full px-4 py-2 border border-slate-300 rounded-lg shadow-sm focus:ring-sky-500 focus:border-sky-500 transition-colors" />
        </div>

        <div class="mb-6">
          <label for="passengers" class="block text-sm font-medium text-slate-700 mb-1">Número de Passageiros:</label>
          <select id="passengers" v-model.number="reservationForm.passengers" @change="updateTotalPrice" required
                  class="w-full px-4 py-2 border border-slate-300 rounded-lg shadow-sm focus:ring-sky-500 focus:border-sky-500 transition-colors">
            <option value="1">1 Passageiro</option>
            <option value="2">2 Passageiros</option>
            <option value="3">3 Passageiros</option>
            <option value="4">4 Passageiros</option>
          </select>
        </div>

        <div class="mb-8 p-4 bg-sky-50 rounded-lg">
          <p class="text-lg font-semibold text-slate-700">Valor por Cabine: <span class="text-sky-600">R$ {{ cruzeiroData.valorCabine.toFixed(2).replace('.', ',') }}</span></p>
          <p class="text-2xl font-bold text-emerald-600 mt-1">Valor Total da Reserva: R$ {{ totalPrice.toFixed(2).replace('.', ',') }}</p>
        </div>

        <button type="submit"
                :disabled="reservationPending"
                class="w-full bg-emerald-500 hover:bg-emerald-600 text-white font-bold py-3 px-6 rounded-lg text-lg transition-colors shadow-md hover:shadow-lg disabled:opacity-50 disabled:cursor-not-allowed">
          <span v-if="reservationPending">Processando Reserva...</span>
          <span v-else>Confirmar Reserva</span>
        </button>
      </form>

      <div v-if="reservationStatus" class="mt-6 p-4 rounded-lg"
           :class="reservationSuccess ? 'bg-green-100 text-green-700' : 'bg-red-100 text-red-700'">
        <h3 class="font-semibold">{{ reservationSuccess ? 'Reserva Confirmada!' : 'Erro na Reserva' }}</h3>
        <p>{{ reservationStatus }}</p>
        <p v-if="reservationDetails && reservationSuccess">ID do Bilhete: {{ reservationDetails.ticketId }}</p>
        <p v-if="reservationDetails && reservationSuccess">Status: {{ reservationDetails.status }}</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
interface CruzeiroParaReserva {
  id: number;
  nome: string;
  valorCabine: number;
}

interface ReservationRequest {
  cruiseId: string;
  customer: string;
  passengers: number;
  totalPrice: number;
}

interface BilheteResponse { // Supondo a estrutura da resposta de sucesso
  ticketId: string;
  reservationId: string;
  customer: string;
  cruiseId: string;
  status: string;
  message: string;
  issuedAt: string; // ou Date
}

const route = useRoute();
const cruzeiroIdRouteParam = computed(() => route.params.id as string);

const API_BASE_URL = 'http://localhost:8080';

const reservationForm = reactive<Omit<ReservationRequest, 'cruiseId' | 'totalPrice'>>({
  customer: '',
  passengers: 1,
});

const totalPrice = ref(0);
const reservationPending = ref(false);
const reservationStatus = ref<string | null>(null);
const reservationSuccess = ref(false);
const reservationDetails = ref<BilheteResponse | null>(null);

// Buscar dados do cruzeiro para obter o valor da cabine
const { data: cruzeiroData, pending: cruzeiroPending, error: cruzeiroError } = await useAsyncData<CruzeiroParaReserva>(
    `cruzeiro-reserva-${cruzeiroIdRouteParam.value}`,
    () => $fetch(`${API_BASE_URL}/cruzeiros/${cruzeiroIdRouteParam.value}`),
    {
      watch: [cruzeiroIdRouteParam],
      onResponse({ response }) {
        if (response._data) {
          updateTotalPriceOnLoad(response._data.valorCabine);
        }
      }
    }
);

function updateTotalPrice() {
  if (cruzeiroData.value) {
    totalPrice.value = reservationForm.passengers * cruzeiroData.value.valorCabine;
  }
}
function updateTotalPriceOnLoad(valorCabine: number) {
  totalPrice.value = reservationForm.passengers * valorCabine;
}


watch(() => reservationForm.passengers, updateTotalPrice);

watch(cruzeiroData, (newData) => {
  if (newData) {
    updateTotalPriceOnLoad(newData.valorCabine);
  }
}, { immediate: true });


async function handleReservation() {
  if (!cruzeiroData.value) {
    reservationStatus.value = "Não foi possível carregar os dados do cruzeiro para a reserva.";
    reservationSuccess.value = false;
    return;
  }

  reservationPending.value = true;
  reservationStatus.value = null;
  reservationSuccess.value = false;
  reservationDetails.value = null;

  const payload: ReservationRequest = {
    cruiseId: String(cruzeiroData.value.id), // Garante que é string
    customer: reservationForm.customer,
    passengers: reservationForm.passengers,
    totalPrice: totalPrice.value,
  };

  try {
    
    const response = await $fetch<BilheteResponse>(`${API_BASE_URL}/reservations`, {
      method: 'POST',
      body: payload,
    });
    console.log("Resposta da reserva:", response);

    if (response.status === 'failed') {
      throw new Error(response.message || 'Falha ao processar a reserva.');
    }

    reservationStatus.value = response.message || 'Reserva processada com sucesso!';
    reservationSuccess.value = true;
    reservationDetails.value = response; // Armazena toda a resposta do bilhete

  } catch (err: any) {
    console.error("Erro na reserva:", err);
    if (err.response && err.response._data) {
      const errorData = err.response._data;
      reservationStatus.value = errorData.details || errorData.error || errorData.message || 'Falha ao processar a reserva.';
    } else {
      reservationStatus.value = err.message || 'Ocorreu um erro desconhecido ao tentar reservar.';
    }
    reservationSuccess.value = false;
  } finally {
    reservationPending.value = false;
  }
}

useHead({
  title: `Reservar ${cruzeiroData.value?.nome || 'Cruzeiro'} - Reserva de Cruzeiros`,
});
</script>