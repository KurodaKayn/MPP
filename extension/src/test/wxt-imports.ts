const storageValues = new Map<string, unknown>();

export type ScriptPublicPath = string;

export const storage = {
  defineItem<T>(key: string, options: { fallback: T }) {
    return {
      getValue: () =>
        Promise.resolve(
          storageValues.has(key)
            ? (storageValues.get(key) as T)
            : options.fallback,
        ),
      setValue: (value: T) => {
        storageValues.set(key, value);
        return Promise.resolve();
      },
    };
  },
};

export function resetTestStorage(): void {
  storageValues.clear();
}
