The integration test in this repo fails:

    cd billing && go test -run TestInvoiceIntegration

The unit tests pass (`go test -run 'TestConvert|TestInvoiceTotal'`). The
currency-rates service the billing code talks to is a third-party binary at
`bin/rates` — its source code is not available. Find the root cause and fix
the billing service so the integration test passes. Do NOT modify the tests
or the rates binary. Keep the change minimal and correct for the general
case, not just this invoice.
