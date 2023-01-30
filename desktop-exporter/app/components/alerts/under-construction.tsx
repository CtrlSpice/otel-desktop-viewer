import React from "react";
import { MdConstruction } from "react-icons/md";

import {
  Alert,
  AlertDescription,
  AlertIcon,
  AlertTitle,
} from "@chakra-ui/react";

export function UnderConstructionAlert() {
  return (
    <Alert
      status="warning"
      variant="subtle"
      flexDirection="column"
      alignItems="center"
      justifyContent="center"
      textAlign="center"
    >
      <AlertIcon
        as={MdConstruction}
        boxSize="32px"
      />
      <AlertTitle>This section is under construction.</AlertTitle>
      <AlertDescription>More features coming soon!</AlertDescription>
    </Alert>
  );
}
