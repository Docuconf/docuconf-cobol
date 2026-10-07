      *> @service demo  @prefix CFG-
       01  DEMO-CONFIG.
      *> Port to listen on
      *> @min 1  @max 100000  @defualt 80
           05  CFG-PORT                PIC 9(5).
      *> Share of orders to sample
      *> @default 0.125
           05  CFG-RATIO               PIC 9V99.
