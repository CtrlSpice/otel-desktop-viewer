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
  let typeOfFieldValue = typeof fieldValue;

  switch (fieldValue) {
    case true:
      fieldValue = "true";
      break;
    case false:
      fieldValue = "false";
      break;
    case null:
      fieldValue = "null";
      break;
    case undefined:
      fieldValue = "undefined";
      break;
    case "":
      fieldValue = '""';
      break;
  }

  return (
    <Box paddingTop={2}>
      <dt>
        <Flex rowGap={2}>
          <Tag
            size="sm"
            variant="outline"
            colorScheme="cyan"
          >
            <TagLabel fontSize="xs">{typeOfFieldValue}</TagLabel>
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
