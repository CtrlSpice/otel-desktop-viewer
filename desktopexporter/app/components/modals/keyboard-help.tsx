import React from "react";
import {
  Flex,
  Kbd,
  Modal,
  ModalBody,
  ModalCloseButton,
  ModalContent,
  ModalHeader,
  ModalOverlay,
  Table,
  TableContainer,
  Tbody,
  Td,
  Th,
  Thead,
  Tr,
} from "@chakra-ui/react";

type KeyboardHelpProps = {
  isOpen: boolean;
  onClose: () => void;
};

// Note: the isNumeric prop is used by Chakra-UI to align numbers in a table
// to the right, and is used here strictly for positioning.
export function KeyboardHelp(props: KeyboardHelpProps) {
  let { isOpen, onClose } = props;
  return (
    <Modal
      onClose={onClose}
      isOpen={isOpen}
      size="2xl"
      isCentered
    >
      <ModalOverlay />
      <ModalContent>
        <ModalHeader>Keyboard Help</ModalHeader>
        <ModalCloseButton />
        <ModalBody paddingBottom="16px">
          <Flex flexDirection="row">
            <TableContainer>
              <Table
                variant="simple"
                size="sm"
              >
                <Thead>
                  <Tr>
                    <Th>Navigation</Th>
                  </Tr>
                </Thead>
                <Tbody>
                  <Tr>
                    <Td>Move up the trace summary list</Td>
                    <Td isNumeric>
                      <Kbd>←</Kbd> or <Kbd>h</Kbd>
                    </Td>
                  </Tr>
                  <Tr>
                    <Td>Move down the trace summary list</Td>
                    <Td isNumeric>
                      <Kbd>→</Kbd> or <Kbd>l</Kbd>
                    </Td>
                  </Tr>
                  <Tr>
                    <Td>Move up the span list</Td>
                    <Td isNumeric>
                      <Kbd>↑</Kbd> or <Kbd>k</Kbd>
                    </Td>
                  </Tr>
                  <Tr>
                    <Td>Move up the trace summary list</Td>
                    <Td isNumeric>
                      <Kbd>↓</Kbd> or <Kbd>j</Kbd>
                    </Td>
                  </Tr>
                </Tbody>
              </Table>
            </TableContainer>
            <TableContainer>
              <Table
                variant="simple"
                size="sm"
              >
                <Thead>
                  <Tr>
                    <Th>Shortcuts</Th>
                  </Tr>
                </Thead>
                <Tbody>
                  <Tr>
                    <Td>Clear all traces</Td>
                    <Td isNumeric>
                      <Kbd>ctrl</Kbd> + <Kbd>l</Kbd>
                    </Td>
                  </Tr>
                  <Tr>
                    <Td>Refresh the page</Td>
                    <Td isNumeric>
                      <Kbd>r</Kbd>
                    </Td>
                  </Tr>
                  <Tr>
                    <Td>Bring up this help dialog</Td>
                    <Td isNumeric>
                      <Kbd>?</Kbd>
                    </Td>
                  </Tr>
                </Tbody>
              </Table>
            </TableContainer>
          </Flex>
        </ModalBody>
      </ModalContent>
    </Modal>
  );
}
