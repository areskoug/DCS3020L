# Ändringar mot vishen/go-chromecast v0.3.4

Kopian är medtagen i repot för att ändringarna nedan behövs. Licensen (Apache-2.0) följer med.
Ändringarna gjordes i `application/application.go`.

1. **`LIVE` i stället för `BUFFERED`** när en webbadress spelas (`play`). Hjälpte inte mot lagget i slutändan men skadar inte.
2. **Panik `send on closed channel`.** `Close` stängde `messageChan` medan `recvMessages` kunde vara på väg att skicka på den, vilket
   tog ned hela processen. Nu skyddas det med `closeMu` och `relay`.
3. **Delad tabell utan lås.** `resultChanMap` lästes av `recvMessages` och skrevs av `sendAndWait` utan lås. Nu skyddas den med `resultMu`.
4. **Svar som tappades.** `sendAndWait` skickade begäran innan svarskanalen fanns, så ett snabbt svar kunde tappas och anropet väntade 5 sekunder.
   Nu registreras kanalen först.
5. **Delat begärans-nummer.** Numret var en global variabel, och nummer skrevs in i delade objekt som `cast.GetStatusHeader`, så anslutningar
   som körde samtidigt kunde skriva över varandras nummer. Nu är räknaren atomär och objektet kopieras (`withRequestID`).
6. **Skydd mot panik** i bibliotekets bakgrundsprocesser (`recoverGoroutine`).

Testerna som återskapar felen finns i `internal/devices/cast_lib_test.go` och körs med `go test -race ./internal/devices`.
