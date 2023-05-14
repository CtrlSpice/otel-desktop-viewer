import { useState, useEffect } from "react";

export const useKeyPress = (targetKeys: string[]) => {
  const [keyPressed, setKeyPressed] = useState(false);

  useEffect(
    () => {
      const downHandler = (event: KeyboardEvent) => {
        event.preventDefault();
        if (targetKeys.includes(event.key)) {
          setKeyPressed(true);
        }
      };

      const upHandler = (event: KeyboardEvent) => {
        if (targetKeys.includes(event.key)) {
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
    // re-run the effect if the targetKeys change.
    [targetKeys, setKeyPressed],
  );
  return keyPressed;
};
