# Samspel Hub

Multiroom-hubb som skickar ett ljudflöde till flera högtalare samtidigt. Körs som Home Assistant OS-add-on.

**Status (steg 1):** enhetsupptäckt (Google Cast via mDNS, Denon HEOS via SSDP), grupper, webbgränssnitt, MP3-ström per grupp och uppspelning av en testton.
Ännu inte klart: AirPlay-mottagare (steg 2), fördröjning per enhet (steg 3).

## Installera i Home Assistant OS
1. Inställningar → Tillägg → Tilläggsbutik → ⋮ → Förråd → lägg till `https://github.com/areskoug/DCS3020L`.
2. Installera **Samspel Hub** och starta. UI: `http://<ha-ip>:8080`.
3. Tryck **Sök igen**, skapa en grupp, tryck **Spela**. Du ska höra en 440 Hz-ton.

## Köra lokalt
Kräver Go 1.25 och ffmpeg.

    cd samspel && go run ./cmd/hub -port 8080

## Okänt / ej testat mot riktiga enheter
- Cast: strömmen skickas som `BUFFERED` med `audio/mpeg`. Live-ström kan kräva annan inställning.
- HEOS: SSDP-söktyp och `play_stream` följer specen, men HEOS-specens versionshistorik är motsägelsefull om URL-uppspelning.
- Add-on-bygget (Dockerfile) är inte provbyggt.
