import { useState, useEffect } from "react";

export const useKeyPress = (targetKey: string) => {
  const [keyPressed, setKeyPressed] = useState(false);

  useEffect(
    () => {
      const downHandler = (event: KeyboardEvent) => {
        if (event.key === targetKey) {
          setKeyPressed(true);
        }
      };

      const upHandler = (event: KeyboardEvent) => {
        if (event.key === targetKey) {
          setKeyPressed(false);
        }
      };

      // attach the listeners to the window.
      window.addEventListener("keydown", downHandler);
      window.addEventListener("keyup", upHandler);

      // remove the listeners when the component is unmounted.
      return () => {
        window.removeEventListener("keydown", downHandler);
        window.removeEventListener("keyup", upHandler);
      };
    },
    // re-run the effect if the targetKey changes.
    [targetKey, setKeyPressed],
  );
  return keyPressed;
};
