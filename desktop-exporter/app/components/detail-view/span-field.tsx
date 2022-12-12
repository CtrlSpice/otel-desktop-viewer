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
};

export function SpanField(props: SpanFieldProps) {
  let { fieldName, fieldValue } = props;
  let fieldNameColour = useColorModeValue("gray.600", "gray.400");

  if (fieldValue == null || fieldValue === "") {
    return null;
  }

  return (
    <Box paddingTop={2}>
      <dt>
        <Flex experimental_spaceX={2}>
          <Tag
            size="sm"
            variant="subtle"
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
