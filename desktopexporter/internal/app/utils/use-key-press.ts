import { useState, useEffect } from "react";
import { ModifierKey } from "../types/ui-types";

export const useKeyPress = (targetKeys: string[]) => {
  let [keyPressed, setKeyPressed] = useState(false);
  useEffect(
    () => {
      const downHandler = (event: KeyboardEvent) => {
        if (event.altKey || event.ctrlKey || event.metaKey || event.shiftKey) {
          return;
        }
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

export const useKeyCombo = (
  modifierKeys: ModifierKey[],
  targetKeys: string[],
) => {
  let [comboPressed, setComboPressed] = useState(false);
  useEffect(() => {
    const downHandler = (event: KeyboardEvent) => {
      let modifiersPressed: boolean = modifierKeys
        .map((modKey) => {
          switch (modKey) {
            case "Alt":
              return event.altKey;
            case "Control":
              return event.ctrlKey;
            case "Meta":
              return event.metaKey;
            case "Shift":
              return event.shiftKey;
            default:
              return false;
          }
        })
        .reduce((accumulator, currentValue) => {
          return accumulator && currentValue;
        });

      if (modifiersPressed && targetKeys.includes(event.key)) {
        event.preventDefault();
        setComboPressed(true);
      }
    };

    const upHandler = (event: KeyboardEvent) => {
      if (
        modifierKeys.map((modKey) => modKey.toString()).includes(event.key) ||
        targetKeys.includes(event.key.toLowerCase())
      ) {
        setComboPressed(false);
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
  }, [modifierKeys, targetKeys, setComboPressed]);
  return comboPressed;
};