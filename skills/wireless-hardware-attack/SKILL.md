---
name: wireless-hardware-attack
description: >-
  Wireless / hardware: WiFi PMKID/WPS/Evil Twin, BLE, Zigbee, NFC, SDR, UART/JTAG/SPI, side-channel, fault injection. Use when attacking WiFi, BLE, RFID, SDR, or hardware interfaces.
metadata:
  tags: [penetration-testing, red-team]
---

## Wireless / Hardware Attacks

```
=== Wireless / Hardware ===
WiFi: aircrack-ng / PMKID (hcxdumptool + hashcat) / WPS reaver / Evil Twin | BLE: gatttool enumerate GATT / unauthenticated read-write / Just Works
Zigbee killerbee | NFC/RFID proxmark3 cloning / MIFARE mfoc | LoRa/SDR rtl-sdr + gnuradio replay
Hardware: UART baud-rate scan to get shell | JTAG/SWD read/write firmware | SPI flash dump | side-channel DPA/timing | fault injection (voltage/clock glitching to skip auth) | binwalk -Me
```
