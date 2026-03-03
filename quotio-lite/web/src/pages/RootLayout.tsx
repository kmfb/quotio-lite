import { Outlet } from '@tanstack/react-router'
import { Badge, Box, Card, Container, Flex, Heading, Text } from '@radix-ui/themes'

export function RootLayout() {
  return (
    <Container size="4" className="page-shell">
      <Flex direction="column" gap="4">
        <Card className="app-header-card">
          <Flex align="center" justify="between" gap="3" wrap="wrap">
            <Box className="app-header">
              <Flex align="center" gap="2" wrap="wrap">
                <Heading size="5">Quotio-Lite</Heading>
                <Badge variant="soft" color="gray">
                  Local
                </Badge>
              </Flex>
              <Text size="2" color="gray">
                Codex account operations and quota visibility
              </Text>
            </Box>
            <Text size="1" color="gray">
              Onboarding · Cleanup · Health Probe · Quota Tracking
            </Text>
          </Flex>
        </Card>
        <Outlet />
      </Flex>
    </Container>
  )
}
