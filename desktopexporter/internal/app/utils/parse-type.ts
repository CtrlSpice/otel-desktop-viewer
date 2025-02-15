export type AttributeType = 
  | "string"
  | "int64"
  | "float64"
  | "boolean"
  | "string[]"
  | "int64[]"
  | "float64[]"
  | "boolean[]"
  | "unknown[]"
  | "unknown"
  | "null";

export function parseAttributeType(value: unknown): AttributeType {
  if (value === null || value === undefined) {
    return "unknown";
  }

  if (Array.isArray(value)) {
    if (value.length === 0) {
      return "unknown";
    }

    const firstElement = value[0];
    if (typeof firstElement === "string") {
      return "string[]";
    }
    if (typeof firstElement === "number") {
      return value.every(num => Number.isInteger(num)) ? "int64[]" : "float64[]";
    }
    if (typeof firstElement === "boolean") {
      return "boolean[]";
    }
    return "unknown[]";
  }

  if (typeof value === "string") {
    return "string";
  }
  if (typeof value === "number") {
    return Number.isInteger(value) ? "int64" : "float64";
  }
  if (typeof value === "boolean") {
    return "boolean";
  }

  return "unknown";
}