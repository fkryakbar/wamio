import { create } from 'zustand';
import type { AccountInfo, ConnectionState, QRCodeEvent, UserInfo } from '../types';

interface AuthState {
  // State
  connectionState: ConnectionState;
  qrCode: string | null;
  userInfo: UserInfo | null;
  accountId: string;
	accounts: AccountInfo[];
  errorMessage: string | null;

  // Actions
  setConnectionState: (state: ConnectionState) => void;
  setQRCode: (qr: QRCodeEvent) => void;
	clearQRCode: () => void;
  setUserInfo: (info: UserInfo | null) => void;
  setAccountId: (id: string) => void;
	setAccounts: (accounts: AccountInfo[]) => void;
  setError: (message: string | null) => void;
  reset: () => void;
}

const initialState = {
  connectionState: 'disconnected' as ConnectionState,
  qrCode: null as string | null,
  userInfo: null as UserInfo | null,
  accountId: '',
	accounts: [] as AccountInfo[],
  errorMessage: null as string | null,
};

export const useAuthStore = create<AuthState>((set) => ({
  ...initialState,

  setConnectionState: (state) =>
    set({ connectionState: state, errorMessage: null }),

  setQRCode: (qr) => {
    if (qr.event === 'code') {
      set({ qrCode: qr.code, connectionState: 'qr_ready' });
    } else if (qr.event === 'success') {
      set({ qrCode: null, connectionState: 'connected' });
    } else if (qr.event === 'timeout') {
      set({ qrCode: null, errorMessage: 'QR Code expired. Tekan tombol untuk mencoba lagi.' });
    }
  },

	clearQRCode: () => set({ qrCode: null }),

  setUserInfo: (info) => set({ userInfo: info }),

  setAccountId: (id) => set({ accountId: id }),

	setAccounts: (accounts) => set({ accounts }),

  setError: (message) => set({ errorMessage: message }),

  reset: () => set(initialState),
}));
