declare module '/wails/runtime.js' {
  export interface WailsEvent<T = unknown> {
    name: string
    data: T
    sender?: string
  }

  export const Browser: {
    OpenURL(url: string): Promise<void>
  }

  export const Events: {
    On<T = unknown>(name: string, callback: (event: WailsEvent<T>) => void): () => void
    Emit<T = unknown>(name: string, data?: T): Promise<boolean>
  }

  export const Window: {
    Close(): Promise<void>
    Focus(): Promise<void>
    Size(): Promise<{ width: number; height: number }>
    SetSize(width: number, height: number): Promise<void>
    SetMinSize(width: number, height: number): Promise<void>
  }
}
