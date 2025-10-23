import { type FieldDefinition, GLOBAL_FIELD } from '@/constants/fields';
import { OPERATORS, type Operator } from '@/constants/operators';
import { type QueryNode, generateId } from './queryTree';

// Token Types For Lexer
type TokenType =
  | 'FIELD'
  | 'OPERATOR'
  | 'VALUE'
  | 'LOGICAL'
  | 'LPAREN'
  | 'RPAREN'
  | 'LBRACKET'
  | 'RBRACKET'
  | 'COMMA'
  | 'EOF';

interface Token {
  type: TokenType;
  value: string;
  position: number;
}

// Tokenizer: Converts Text Into Tokens
class Lexer {
  private input: string;
  private position: number = 0;
  private current: string = '';

  constructor(input: string) {
    this.input = input;
    this.current = input[0] || '';
  }

  // Advance To Next Character
  private advance(): void {
    this.position++;
    this.current = this.input[this.position] || '';
  }

  // Peek At Next Character Without Advancing
  private peek(offset: number = 1): string {
    return this.input[this.position + offset] || '';
  }

  // Skip Whitespace
  private skipWhitespace(): void {
    while (this.current && /\s/.test(this.current)) {
      this.advance();
    }
  }

  // Read A String (quoted or unquoted)
  private readString(): string {
    let result = '';
    const quote = this.current;

    if (quote === '"' || quote === "'" || quote === '`') {
      // Quoted string (supports single, double, and backtick for template literals)
      this.advance(); // Skip opening quote
      while (this.current && this.current !== quote) {
        if (this.current === '\\') {
          this.advance();
          const escaped: string = this.current;
          if (escaped) {
            // Handle escape sequences
            if (escaped === 'n') {
              result += '\n';
            } else if (escaped === 't') {
              result += '\t';
            } else if (escaped === 'r') {
              result += '\r';
            } else if (escaped === '\\') {
              result += '\\';
            } else if (escaped === quote) {
              result += quote;
            } else {
              result += escaped;
            }
            this.advance();
          }
        } else {
          result += this.current;
          this.advance();
        }
      }
      if (this.current === quote) {
        this.advance(); // Skip closing quote
      }
    } else {
      // Unquoted string - read until whitespace or special char
      while (
        this.current &&
        !/[\s()[\],]/.test(this.current) &&
        !this.isOperatorStart()
      ) {
        result += this.current;
        this.advance();
      }
    }

    return result;
  }

  // Check If Current Position Starts An Operator
  private isOperatorStart(): boolean {
    const twoChar = this.current + this.peek();
    return (
      this.current === ':' ||
      this.current === '=' ||
      this.current === '!' ||
      this.current === '>' ||
      this.current === '<' ||
      this.current === '~' ||
      this.current === '^' ||
      this.current === '$' ||
      twoChar === '^=' ||
      twoChar === '$='
    );
  }

  // Read An Operator
  private readOperator(): string {
    const twoChar = this.current + this.peek();

    // Check two-character operators first
    if (['!=', '!~', '>=', '<=', '=~', '^=', '$='].includes(twoChar)) {
      this.advance();
      this.advance();
      return twoChar;
    }

    // Single character operators
    const op = this.current;
    this.advance();
    return op;
  }

  // Read A Keyword Or Identifier
  private readKeywordOrIdentifier(): string {
    let result = '';
    while (this.current && /[a-zA-Z0-9_.]/.test(this.current)) {
      result += this.current;
      this.advance();
    }
    return result;
  }

  // Get Next Token
  public nextToken(): Token {
    this.skipWhitespace();

    const position = this.position;

    // End of input
    if (!this.current) {
      return { type: 'EOF', value: '', position };
    }

    // Parentheses
    if (this.current === '(') {
      this.advance();
      return { type: 'LPAREN', value: '(', position };
    }
    if (this.current === ')') {
      this.advance();
      return { type: 'RPAREN', value: ')', position };
    }

    // Brackets for arrays
    if (this.current === '[') {
      this.advance();
      return { type: 'LBRACKET', value: '[', position };
    }
    if (this.current === ']') {
      this.advance();
      return { type: 'RBRACKET', value: ']', position };
    }

    // Comma
    if (this.current === ',') {
      this.advance();
      return { type: 'COMMA', value: ',', position };
    }

    // Operator
    if (this.isOperatorStart()) {
      const op = this.readOperator();
      return { type: 'OPERATOR', value: op, position };
    }

    // Quoted string (single, double, or backtick)
    if (this.current === '"' || this.current === "'" || this.current === '`') {
      const value = this.readString();
      return { type: 'VALUE', value, position };
    }

    // Keyword or identifier
    const word = this.readKeywordOrIdentifier();
    const upperWord = word.toUpperCase();

    // Check if it's a logical operator (case-insensitive)
    if (upperWord === 'AND' || upperWord === 'OR') {
      return { type: 'LOGICAL', value: upperWord, position };
    }

    // Check if it's a keyword operator (case-insensitive: IN, CONTAINS, REGEXP, NOT IN, NOT CONTAINS)
    if (
      upperWord === 'IN' ||
      upperWord === 'CONTAINS' ||
      upperWord === 'REGEXP'
    ) {
      return { type: 'OPERATOR', value: upperWord, position };
    }

    // Handle "NOT IN" and "NOT CONTAINS" as special cases (case-insensitive)
    if (upperWord === 'NOT') {
      this.skipWhitespace();
      const next = this.readKeywordOrIdentifier();
      const upperNext = next.toUpperCase();
      if (upperNext === 'IN') {
        return { type: 'OPERATOR', value: 'NOT IN', position };
      }
      if (upperNext === 'CONTAINS') {
        return { type: 'OPERATOR', value: 'NOT CONTAINS', position };
      }
      throw new Error(`Unexpected token: NOT ${next}`);
    }

    // Check for null values (case-insensitive)
    if (upperWord === 'NULL' || upperWord === 'NIL') {
      return { type: 'VALUE', value: 'NULL', position };
    }

    // Otherwise it's a field or value
    // We'll determine which during parsing
    return { type: 'FIELD', value: word, position };
  }

  // Tokenize entire input
  public tokenize(): Token[] {
    let tokens: Token[] = [];
    let token = this.nextToken();

    while (token.type !== 'EOF') {
      tokens.push(token);
      token = this.nextToken();
    }

    return tokens;
  }
}

// Parser: Converts Tokens Into QueryNode Tree
class Parser {
  private tokens: Token[];
  private position: number = 0;
  private availableFields: FieldDefinition[];

  constructor(tokens: Token[], availableFields: FieldDefinition[]) {
    this.tokens = tokens;
    this.availableFields = availableFields;
  }

  // Get Current Token
  private current(): Token | undefined {
    return this.tokens[this.position];
  }

  // Advance To Next Token
  private advance(): void {
    this.position++;
  }

  // Check If Current Token Matches Type
  private check(type: TokenType): boolean {
    const token = this.current();
    return token !== undefined && token.type === type;
  }

  // Consume Token Of Expected Type
  private consume(type: TokenType, message: string): Token {
    const token = this.current();
    if (!token || token.type !== type) {
      throw new Error(message);
    }
    this.advance();
    return token;
  }

  // Find Field By Name
  private findField(name: string): FieldDefinition | undefined {
    // First, check if it's a known field or attribute
    let field = this.availableFields.find(
      f => f.name.toLowerCase() === name.toLowerCase()
    );
    if (field) return field;

    // If not found, create a user-defined attribute for general scope
    return {
      name,
      type: 'string', // Default to string for user-defined
      searchScope: 'attribute',
      attributeScope: 'general',
      operators: [
        OPERATORS.EQUALS,
        OPERATORS.NOT_EQUALS,
        OPERATORS.CONTAINS,
        OPERATORS.NOT_CONTAINS,
        OPERATORS.STARTS_WITH,
        OPERATORS.ENDS_WITH,
        OPERATORS.REGEX,
      ],
      description: `User-defined attribute: ${name}`,
    };
  }

  // Find Operator By Symbol
  private findOperator(symbol: string): Operator | undefined {
    for (let op of Object.values(OPERATORS)) {
      if (op.symbol === symbol) {
        return op;
      }
    }
    return undefined;
  }

  // Parse: query → expression
  public parse(): QueryNode | null {
    if (!this.current()) {
      return null;
    }
    return this.parseExpression();
  }

  // Parse: expression → term ( ( "AND" | "OR" ) term )*
  private parseExpression(): QueryNode {
    let left = this.parseTerm();

    while (this.check('LOGICAL')) {
      const operatorToken = this.current()!;
      const operator = operatorToken.value as 'AND' | 'OR';
      this.advance();

      const right = this.parseTerm();

      // Create group node
      left = {
        id: generateId(),
        type: 'group',
        group: {
          operator,
          children: [left, right],
        },
      };
    }

    return left;
  }

  // Parse: term → "(" expression ")" | condition
  private parseTerm(): QueryNode {
    // Parenthesized expression
    if (this.check('LPAREN')) {
      this.advance(); // consume '('
      const expr = this.parseExpression();
      this.consume('RPAREN', 'Expected closing parenthesis');
      return expr;
    }

    // Condition
    return this.parseCondition();
  }

  // Parse: condition → field operator value
  private parseCondition(): QueryNode {
    // Parse field
    const fieldToken = this.consume('FIELD', 'Expected field name');
    const field = this.findField(fieldToken.value);

    if (!field) {
      throw new Error(`Unknown field: ${fieldToken.value}`);
    }

    // Parse operator
    const operatorToken = this.consume('OPERATOR', 'Expected operator');
    const operator = this.findOperator(operatorToken.value);

    if (!operator) {
      throw new Error(`Unknown operator: ${operatorToken.value}`);
    }

    // Validate operator is allowed for this field
    if (!field.operators.includes(operator)) {
      throw new Error(
        `Operator '${operator.symbol}' is not valid for field '${field.name}'`
      );
    }

    // Parse value
    let value: string = '';

    // Check for array value (IN, NOT IN)
    if (this.check('LBRACKET')) {
      value = this.parseArray();
    } else {
      const valueToken = this.current();
      if (
        !valueToken ||
        (valueToken.type !== 'VALUE' && valueToken.type !== 'FIELD')
      ) {
        throw new Error('Expected value');
      }
      value = valueToken.value;
      this.advance();
    }

    // Normalize null values
    if (value.toUpperCase() === 'NULL' || value.toUpperCase() === 'NIL') {
      value = 'NULL';
    }

    // Create condition node
    return {
      id: generateId(),
      type: 'condition',
      query: {
        field,
        operator,
        value,
      },
    };
  }

  // Parse: array → "[" value ( "," value )* "]"
  private parseArray(): string {
    this.consume('LBRACKET', 'Expected [');

    let values: string[] = [];

    // Parse first value
    if (!this.check('RBRACKET')) {
      const valueToken = this.current();
      if (
        !valueToken ||
        (valueToken.type !== 'VALUE' && valueToken.type !== 'FIELD')
      ) {
        throw new Error('Expected value in array');
      }
      values.push(valueToken.value);
      this.advance();

      // Parse remaining values
      while (this.check('COMMA')) {
        this.advance(); // consume comma
        const valueToken = this.current();
        if (
          !valueToken ||
          (valueToken.type !== 'VALUE' && valueToken.type !== 'FIELD')
        ) {
          throw new Error('Expected value after comma');
        }
        values.push(valueToken.value);
        this.advance();
      }
    }

    this.consume('RBRACKET', 'Expected ]');

    return `[${values.join(',')}]`;
  }
}

// Create Global Text Search Query
function createGlobalTextSearch(input: string): QueryNode {
  return {
    id: generateId(),
    type: 'condition',
    query: {
      field: GLOBAL_FIELD,
      operator: OPERATORS.CONTAINS,
      value: input.trim(),
    },
  };
}

// Main Parse Function
export function parseQuery(
  input: string,
  availableFields: FieldDefinition[]
): QueryNode | null {
  if (!input.trim()) {
    return null;
  }

  try {
    const lexer = new Lexer(input);
    const tokens = lexer.tokenize();
    const parser = new Parser(tokens, availableFields);
    return parser.parse();
  } catch (error) {
    // If structured parsing fails, check if it's just plain text
    const trimmedInput = input.trim();

    // If it's plain text (no operators, no colons), treat as full-text search
    if (
      trimmedInput &&
      !trimmedInput.includes(':') &&
      !trimmedInput.includes('=') &&
      !trimmedInput.includes('~') &&
      !trimmedInput.includes('>') &&
      !trimmedInput.includes('<') &&
      !trimmedInput.includes('!') &&
      !trimmedInput.includes('[') &&
      !trimmedInput.includes('(') &&
      !trimmedInput.match(/\b(AND|OR|IN|NOT)\b/i)
    ) {
      return createGlobalTextSearch(trimmedInput);
    }

    // Re-throw the original parse error
    throw new Error(
      error instanceof Error ? error.message : 'Failed to parse query'
    );
  }
}
