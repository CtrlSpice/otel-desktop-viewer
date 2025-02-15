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
  fieldType: string;
  fieldName: string;
  fieldValue: string;
  hidden?: boolean;
};

export function SpanField(props: SpanFieldProps) {
  let { fieldType, fieldName, fieldValue, hidden } = props;
  let fieldNameColour = useColorModeValue("gray.600", "gray.400");

  if (hidden) {
    return null;
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
            <TagLabel fontSize="xs">{fieldType}</TagLabel>
          </Tag>
          <Text
            textColor={fieldNameColour}
            fontSize="sm"
            marginLeft={2}
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
