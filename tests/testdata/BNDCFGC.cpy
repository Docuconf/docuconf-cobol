000100* @SERVICE BOUNDS-TEST  @PREFIX BC-  @PROGRAM BNDCFG              BNDCFGC
000200* Worker count                                                    BNDCFGC
000300* @MIN 1  @MAX 64                                                 BNDCFGC
000400     05  BC-WORKER-COUNT     PIC S9(4) VALUE 4.                   BNDCFGC
000500* Share to sample                                                 BNDCFGC
000600* @MIN 0.1  @MAX 0.9                                              BNDCFGC
000700     05  BC-RATIO            PIC 9V99 VALUE 0.5.                  BNDCFGC
000800* Request timeout                                                 BNDCFGC
000900* @UNIT ms  @MIN 1s  @MAX 5m                                      BNDCFGC
001000     05  BC-TIMEOUT          PIC 9(7) VALUE 30000.                BNDCFGC
001100     05  BC-PORT  PIC 9(5).  *> Metrics port @DEFAULT 80          BNDCFGC
001200* Verbose output                                                  BNDCFGC
001300* @TYPE bool                                                      BNDCFGC
001400     05  BC-VERBOSE          PIC X VALUE 'Y'.                     BNDCFGC
001500* Shards owned                                                    BNDCFGC
001600* @MIN-ITEMS 1  @ITEM-MIN 0  @ITEM-MAX 9  @COUNT BC-SHARD-COUNT   BNDCFGC
001700     05  BC-SHARDS           PIC S9(4) OCCURS 3.                  BNDCFGC
001800     05  BC-SHARD-COUNT      PIC 9.                               BNDCFGC
