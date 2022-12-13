import React from "react";
import {
  Flex,
  Box,
  Tag,
  Text,
  TagLabel,
  useColorModeValue,
} from "@chakra-ui/react";

type SpanFieldProps = {
  fieldName: string;
  fieldValue: number | string | boolean | null;
  hidden?: boolean;
};

export function SpanField(props: SpanFieldProps) {
  let { fieldName, fieldValue, hidden } = props;
  let fieldNameColour = useColorModeValue("gray.600", "gray.400");

  if (hidden) {
    return null;
  }

  switch (fieldValue) {
    case null:
      fieldValue = "null";
      break;
    case undefined:
      fieldValue = "undelined";
    case "":
      fieldValue = '""';
  }

  return (
    <Box paddingTop={2}>
      <dt>
        <Flex experimental_spaceX={2}>
          <Tag
            size="sm"
            variant="outline"
            colorScheme="cyan"
          >
            <TagLabel fontSize="xs">{typeof fieldValue}</TagLabel>
          </Tag>
          <Text
            textColor={fieldNameColour}
            fontSize="sm"
          >
            {fieldName}
          </Text>
        </Flex>
      </dt>
      <dd>
        <Text
          fontSize="md"
          paddingY={2}
        >
          {fieldValue}
        </Text>
      </dd>
    </Box>
  );
}
