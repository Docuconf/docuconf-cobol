      *> docuconf runtime: working storage for a generated loader.
      *> docuconf-cobol generate inlines it into every loader, so a
      *> loader compiles on its own. Valid in fixed and free format.
       01  DC-WORK.
           05  DC-NAME             PIC X(64).
           05  DC-NAME-LEN         PIC 9(4) COMP-5.
           05  DC-RAW              PIC X(8192).
           05  DC-LEN              PIC 9(5) COMP-5.
           05  DC-SET              PIC X.
           05  DC-OK               PIC X.
           05  DC-CODE             PIC X(24).
           05  DC-MSG              PIC X(160).
           05  DC-PROBLEMS         PIC 9(4) COMP-5.
           05  DC-PROBLEMS-AT      PIC 9(4) COMP-5.
           05  DC-INT              PIC S9(19).
           05  DC-BOOL             PIC X.
           05  DC-NS               PIC S9(24).
           05  DC-CHK              PIC S9(24)V9(9).
           05  DC-UNIT-NS          PIC 9(18).
           05  DC-ENC              PIC X(8).
           05  DC-SEP              PIC X(16).
           05  DC-SEP-LEN          PIC 9(4) COMP-5.
           05  DC-I                PIC 9(5) COMP-5.
           05  DC-J                PIC 9(5) COMP-5.
           05  DC-K                PIC 9(5) COMP-5.
           05  DC-P                PIC 9(5) COMP-5.
           05  DC-Q                PIC 9(5) COMP-5.
           05  DC-C                PIC X.
           05  DC-NEG              PIC X.
           05  DC-TMP              PIC X(64).
           05  DC-TMP-LEN          PIC 9(4) COMP-5.
           05  DC-UNIT             PIC X(8).
           05  DC-NUM              PIC S9(24)V9(12).
           05  DC-DAYS             PIC S9(12).
           05  DC-PARTS            PIC 9(4) COMP-5.
           05  DC-PART             PIC X(64) OCCURS 4.
           05  DC-IN-TIME          PIC X.
           05  DC-HEX              PIC X(4).
           05  DC-CP               PIC 9(9) COMP-5.
           05  DC-CP2              PIC 9(9) COMP-5.
           05  DC-BYTE             PIC 9(4) COMP-5.
           05  DC-BASE             PIC X(64).
           05  DC-IDX-ED           PIC Z(4)9.
           05  DC-ROOT             PIC X(4096).
           05  DC-ROOT-LEN         PIC 9(5) COMP-5.
           05  DC-PATH             PIC X(8192).
           05  DC-ITEM-COUNT       PIC 9(5) COMP-5.
           05  DC-ITEM-LIMIT       PIC 9(5) COMP-5.
           05  DC-ITEMS            OCCURS 1000.
               10  DC-ITEM         PIC X(1024).
               10  DC-ITEM-LEN     PIC 9(5) COMP-5.
               10  DC-ITEM-KIND    PIC X.
