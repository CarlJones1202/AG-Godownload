const favoriteStateChangedEvent = 'favorite-state-changed';

export function notifyFavoriteStateChanged(imageId: number, isFavorite: boolean) {
  window.dispatchEvent(new CustomEvent(favoriteStateChangedEvent, {
    detail: { imageId, isFavorite },
  }));
}

export function subscribeToFavoriteStateChanges(
  listener: (imageId: number, isFavorite: boolean) => void,
) {
  const handleEvent = (event: Event) => {
    const detail = (event as CustomEvent<{ imageId: number; isFavorite: boolean }>).detail;
    listener(detail.imageId, detail.isFavorite);
  };
  window.addEventListener(favoriteStateChangedEvent, handleEvent);
  return () => window.removeEventListener(favoriteStateChangedEvent, handleEvent);
}
