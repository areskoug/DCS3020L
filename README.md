# Samspel Hub

Multiroom-hubb för Home Assistant OS. iPhone spelar via AirPlay till hubben, som skickar ljudet vidare till
flera högtalare samtidigt: Google Cast (Google Home, Chromecast) och Denon HEOS. Skriven i Go.

```
iPhone ──AirPlay──▶ shairport-sync (en per grupp och högtalare) ──PCM──▶ ffmpeg ──MP3──▶ HTTP-ström
                                                                                          │
                                       Google Cast ◀── hämtar strömmen ◀─────────────────┤
                                       Denon HEOS  ◀── hämtar strömmen (play_stream) ◀───┘
```

## Funktioner
- **Grupper:** skapa grupper av högtalare i webbgränssnittet. Varje grupp syns som en egen AirPlay-högtalare.
- **Varje högtalare går också att välja** i AirPlay-menyn, under sitt eget namn.
- **Synk:** Cast och HEOS buffrar olika länge, så varje enhet i en grupp har en egen fördröjning (i sekunder) som du ställer in efter gehör. Testsignalen (ett dubbelpip och ett enkelpip var åttonde sekund) är gjord för det: den upprepas inte varje sekund, så du hör även hela sekunders skillnad. Fördröjningen görs genom att hubben skickar tystnad framför ljudet. Alla högtalare får dessutom 3 sekunder tystnad som förråd, annars tar bufferten slut direkt vid start och högtalaren stannar och väntar flera gånger, vilket lägger på lagg.
- En HEOS-högtalare i en grupp som också har en Cast-högtalare får 0,05 s fördröjning som standard (med samma förråd spelade HEOS lite tidigt). Ett eget värde, även 0, gäller alltid före standardvärdet.
- Provade värden (WAV): Cast 5 s förråd och HEOS 3 s förråd + 2,05 s fördröjning gav synk inom ~50 ms och ~5 s lagg. Med 3 s förråd stannade Cast ibland och väntade, och HEOS förblev tyst. Från 4–4,5 s fungerade båda, så alla högtalare har 5 s som standard.
- **Volym:** telefonens volymreglage sätter högtalarnas egen volym direkt, med justerbar nivå per rum (balans).
- **Spelas nu:** titel, artist, album och omslag visas i gränssnittet (visar vad telefonen spelar just nu, alltså före högtalarnas buffer).
- Enheter upptäcks vid start och var tionde minut, och sparas.

## Installera
1. Home Assistant: Inställningar → Tillägg → Tilläggsbutik → ⋮ → Förråd → lägg till `https://github.com/areskoug/DCS3020L`.
2. Installera **Samspel Hub** och starta. Gränssnittet finns på `http://<ha-ip>:8080`.
3. Skapa en grupp, tryck **Testton** och ställ in fördröjningen per enhet tills pipen låter som ett enda ljud.
   Fördröjningen gäller först när gruppen startas om.
4. Välj gruppens eller högtalarens namn i AirPlay-menyn på telefonen.

HA läser den gren som är förvald i GitHub. Höj `version` i `samspel/config.yaml` för att HA ska erbjuda en uppdatering.

## Släppa en ny version
1. Höj `version` i `samspel/config.yaml`.
2. Lägg en rubrik `## <version>` överst i `samspel/CHANGELOG.md`. HA visar filen på tilläggets sida.
3. Kör `cd samspel && go test ./...`. Testerna i `internal/addon` kontrollerar `config.yaml` mot det format Supervisor kräver, att ändringsloggen har en post för versionen och att Dockerfilen bara kopierar filer som finns.
   En felaktig `config.yaml` gör att HA tyst döljer tillägget, och det syns bara i Supervisor-loggen (så hände det med 0.6.5).
4. Pusha, och tryck sedan på ⋮ → Kontrollera efter uppdateringar i tilläggsbutiken. Bygget tar några minuter på en Pi.

## Begränsningar
- **Fördröjning:** ljudet kommer ~3–10 s efter telefonen, beroende på högtalare, och paus, låtbyte och play märks lika sent.
  Mätt innan förrådet infördes: Cast ~26 s med MP3 och ~18 s med WAV, HEOS ~10 s. Cast loggar `BUFFERING` i loggen när den stannar och väntar.
- Ljudet kodas som MP3 192 kbit/s, eller som okomprimerad WAV (valbart per grupp, testas för lägre lagg). Fördröjningarna sparas separat per format.
- AirPlay 1 (Alpines shairport-sync saknar AirPlay 2 och nqptp). Räcker för musik från iPhone.
- Om en högtalare ingår i en aktiv grupp och du spelar till den enskilt vinner den som startade senast.
- Hubben har ingen inloggning. Den ska bara nås från det lokala nätet.

## Utveckling
Kräver Go 1.25 och ffmpeg.

    cd samspel
    go test ./...
    go run ./cmd/hub -port 8080        # utan shairport-sync fungerar allt utom AirPlay

- `internal/devices`: upptäckt och styrning (Cast via mDNS, HEOS via SSDP och CLI på port 1255).
- `internal/stream`: ffmpeg-kodning, tidsstämplade fördröjningar och testsignal.
- `internal/airplay`: en shairport-sync-process per grupp, ljud- och metadata-pipes.
- `internal/hub`: grupper, enkelgrupper per enhet, volym och uppspelning.
- `internal/web`: API, ljudströmmar och gränssnitt.
- `samspel/third_party/go-chromecast`: kopia av [vishen/go-chromecast](https://github.com/vishen/go-chromecast) (Apache-2.0). Enda ändringen är att strömmen skickas som `LIVE`.
