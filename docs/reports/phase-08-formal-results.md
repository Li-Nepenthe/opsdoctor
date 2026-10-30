# 评测记录 phase08-formal-20261029

这张表来自自建靶场。请按批次配置区分试跑和正式评测；每个场景样本很少，不能代表生产准确率。OpenAI 金额是用量账本，实际扣费由用户在平台核对。

批次 token：8475961 / 30000000（含未确认预留）。

| 场景 | 轮次 | 组 | 诊断 | 状态 | 组件一致 | 轮数 | 工具 | 输入 / 输出 token | 账本费用 | 耗时秒 | 提交方式 | 混合模型 |
|---|---:|---|---|---|---|---:|---:|---:|---:|---:|---|---|
| S01 | 1 | E0 | 43ea79e5bbbf80a61aa4226c37a8086f | succeeded | false | 1 | 0 | 3782 / 318 | USD 0.006317 | 6.170 | voluntary | false |
| S01 | 1 | E1 | 13bb2418f09b64ecc5e3e7d36de4ac19 | succeeded | true | 10 | 20 | 168135 / 4679 | CNY 0.045107 | 31.058 | voluntary | false |
| S01 | 1 | E1 | 86fc3d26d758ec7e7c6cb2b2786091f0 | succeeded | true | 5 | 17 | 53353 / 1104 | USD 0.024287 | 26.168 | voluntary | false |
| S01 | 1 | E2 | 09249c56a5e36706b90e4f5ace777fd9 | succeeded | true | 5 | 16 | 50248 / 1067 | USD 0.022363 | 22.396 | voluntary | false |
| S01 | 1 | E3 | 6fc54e92d0e82a139fcff446076c357f | succeeded | true | 5 | 18 | 58943 / 1240 | USD 0.028330 | 26.735 | voluntary | false |
| S01 | 2 | E2 | ca5e26cab979e55fcee28278f6f02a0b | succeeded | true | 5 | 21 | 64814 / 1396 | USD 0.030577 | 27.649 | voluntary | false |
| S01 | 2 | E1 | 9872da4ba3e43a84c5670c0356402464 | succeeded | true | 7 | 18 | 89880 / 2953 | CNY 0.030445 | 21.797 | voluntary | false |
| S01 | 2 | E3 | d9cc55a57cbfb5cb80b219e107cdf6dd | succeeded | true | 4 | 15 | 40008 / 1065 | USD 0.023112 | 20.828 | voluntary | false |
| S01 | 2 | E1 | ed2ad16f3d420ba6a204d05c29e9dd68 | succeeded | true | 4 | 14 | 48118 / 1033 | USD 0.027129 | 20.622 | voluntary | false |
| S01 | 2 | E0 | af529b5a6daa22c777ad2388355a28e6 | succeeded | false | 1 | 0 | 3691 / 330 | USD 0.006263 | 5.191 | voluntary | false |
| S01 | 3 | E1 | d105a642265f8efc4df415ef72f7391d | succeeded | true | 4 | 16 | 43718 / 1259 | USD 0.026261 | 23.031 | voluntary | false |
| S01 | 3 | E2 | f8d8717950df40b111171df04545ca25 | succeeded | true | 5 | 15 | 53135 / 1044 | USD 0.023599 | 21.489 | voluntary | false |
| S01 | 3 | E0 | ec6824925b4ec8703809a52a0855a9d4 | succeeded | false | 1 | 0 | 3781 / 316 | USD 0.006306 | 5.142 | voluntary | false |
| S01 | 3 | E3 | 907c421b516bfb6c91b6755b94e9961d | succeeded | true | 4 | 15 | 35024 / 1166 | USD 0.021326 | 22.525 | voluntary | false |
| S01 | 3 | E1 | d04dda7a18b97a9aee3d98eee8d1a8b9 | succeeded | true | 6 | 19 | 70985 / 2767 | CNY 0.029496 | 19.531 | voluntary | false |
| S02 | 1 | E1 | 28edfe629eac145155ffeb450c1f4b51 | succeeded | false | 16 | 31 | 270899 / 4196 | CNY 0.067793 | 37.977 | budget | false |
| S02 | 1 | E0 | 65991e296ccd8deed64ecb737adcee19 | succeeded | false | 1 | 0 | 3689 / 364 | USD 0.006431 | 7.170 | voluntary | false |
| S02 | 1 | E3 | 56f0dd7588d2de040b88b595221f60a2 | succeeded | true | 4 | 14 | 37546 / 1052 | USD 0.022490 | 20.551 | voluntary | false |
| S02 | 1 | E2 | dfeb898ba4ebf7be2f9e115c969aa69c | succeeded | true | 5 | 20 | 60286 / 1410 | USD 0.030340 | 26.586 | voluntary | false |
| S02 | 1 | E1 | 0b48d46b2114c1fb736bd0ced94ce03f | succeeded | true | 4 | 12 | 34281 / 1064 | USD 0.019355 | 19.271 | voluntary | false |
| S02 | 2 | E1 | 34979855a77654addc99d74fbee17f7e | succeeded | true | 9 | 18 | 111000 / 2801 | CNY 0.029257 | 22.201 | voluntary | false |
| S02 | 2 | E1 | 345757e2c3df755da57149656b4ffa52 | succeeded | true | 4 | 16 | 36335 / 1137 | USD 0.022270 | 22.162 | voluntary | false |
| S02 | 2 | E0 | 6251bc9fb1dbd2c53c2bf9e528b5b351 | succeeded | false | 1 | 0 | 3690 / 329 | USD 0.006257 | 5.644 | voluntary | false |
| S02 | 2 | E2 | 919dd8f9aae516e945fb04b7fe019a28 | succeeded | true | 4 | 14 | 37299 / 1066 | USD 0.021205 | 21.100 | voluntary | false |
| S02 | 2 | E3 | 4368ed8040bb9abc50273574f172fe52 | cancelled（可靠性；不计入诊断正确率） | false | 3 | 12 | 11664 / 272 | USD 0.049762 | 14.965 | 无报告 | false |
| S02 | 2 | E3 | a9a8ee06b65bbc2e469a2a1d0f2388f3 | succeeded（一次补跑） | true | 4 | 13 | 36217 / 1155 | USD 0.022159 | 23.564 | voluntary | false |
| S02 | 3 | E1 | a8bb6f3151606704d5a0768df6d17345 | succeeded | true | 16 | 25 | 192474 / 3244 | CNY 0.050914 | 30.570 | budget | false |
| S02 | 3 | E3 | 912ddcabd8a176bbfd17184193f75afc | succeeded | true | 5 | 21 | 55755 / 1415 | USD 0.028322 | 26.763 | voluntary | false |
| S02 | 3 | E2 | 5c98f1bfd26169d36057da52a8e61c68 | succeeded | true | 4 | 14 | 38392 / 1185 | USD 0.022687 | 21.816 | voluntary | false |
| S02 | 3 | E0 | 6049d98c4aa0e4dd143f0902065b8d05 | succeeded | false | 1 | 0 | 3688 / 301 | USD 0.006115 | 5.000 | voluntary | false |
| S02 | 3 | E1 | f7ff769ae87b2c5120ab269df6335c3d | succeeded | true | 6 | 22 | 67883 / 1308 | USD 0.026799 | 25.694 | voluntary | false |
| S03 | 1 | E3 | 741de2b174cf6c8c15aeaea938d479eb | succeeded | true | 5 | 21 | 56762 / 1378 | USD 0.028023 | 25.400 | voluntary | false |
| S03 | 1 | E1 | 3b11deff21b754d931b13717f1ec6d14 | succeeded | true | 14 | 23 | 225717 / 8561 | CNY 0.060142 | 45.514 | voluntary | false |
| S03 | 1 | E2 | 894a017a149996a3bd0341f75ee7cfbe | succeeded | true | 5 | 20 | 52080 / 1293 | USD 0.025505 | 25.806 | voluntary | false |
| S03 | 1 | E1 | c66e76c78dc793a71251212f58504757 | succeeded | true | 5 | 19 | 51367 / 1212 | USD 0.025124 | 22.544 | voluntary | false |
| S03 | 1 | E0 | 4e2331a3602d727481194ac22ce9261f | succeeded | false | 1 | 0 | 3613 / 296 | USD 0.005996 | 5.030 | voluntary | false |
| S03 | 2 | E1 | f9efa10edca6a50c394e85b7a6a4f2ac | succeeded | true | 13 | 30 | 187113 / 3472 | CNY 0.037057 | 27.405 | voluntary | false |
| S03 | 2 | E1 | 44b4bb284a99bb0a46c67c53ea32ee3f | succeeded | true | 4 | 14 | 38812 / 940 | USD 0.022640 | 18.449 | voluntary | false |
| S03 | 2 | E0 | 5c39eaf93a2b0f25f92d65aacdbf44a7 | succeeded | false | 1 | 0 | 3614 / 290 | USD 0.005967 | 4.663 | voluntary | false |
| S03 | 2 | E2 | 0a41937b4185b279826c9fe945c50fa4 | succeeded | true | 4 | 12 | 36952 / 965 | USD 0.021685 | 18.323 | voluntary | false |
| S03 | 2 | E3 | 872e9daa944b023451ad80d23a12d544 | succeeded | true | 4 | 15 | 37986 / 1138 | USD 0.022690 | 20.244 | voluntary | false |
| S03 | 3 | E2 | c3be93fa9bc6a098d56c364058f26d29 | succeeded | true | 5 | 19 | 51128 / 1169 | USD 0.024740 | 24.555 | voluntary | false |
| S03 | 3 | E0 | 8787ba82bd7b4ae82b6fe21ded17070c | succeeded | false | 1 | 0 | 3613 / 308 | USD 0.006056 | 5.218 | voluntary | false |
| S03 | 3 | E1 | c30306507664f1ce1afaac795492665a | succeeded | true | 4 | 14 | 39744 / 987 | USD 0.022655 | 18.013 | voluntary | false |
| S03 | 3 | E3 | f4c5d5757cda748fc5f2e42fd9c01f05 | succeeded | true | 5 | 19 | 58437 / 1293 | USD 0.029141 | 23.966 | voluntary | false |
| S03 | 3 | E1 | 0c232571bb8a197d5057acfaac72c734 | succeeded | true | 17 | 31 | 273050 / 5667 | CNY 0.067927 | 40.446 | budget | false |
| S04 | 1 | E1 | 5e6a6339228d61799338b9de8447f5b0 | succeeded | true | 17 | 33 | 308751 / 8503 | CNY 0.086624 | 50.709 | budget | false |
| S04 | 1 | E0 | b8c8eb40818365cdafc1d3de87cb2899 | succeeded | false | 1 | 0 | 3607 / 240 | USD 0.005708 | 4.658 | voluntary | false |
| S04 | 1 | E3 | 7352595519813e99589ff374fcee18a3 | succeeded | true | 4 | 19 | 38008 / 1230 | USD 0.024050 | 22.114 | voluntary | false |
| S04 | 1 | E2 | 26f67b71d28bdf343d22f79415cd9e17 | succeeded | true | 4 | 17 | 45674 / 1021 | USD 0.026643 | 19.897 | voluntary | false |
| S04 | 1 | E1 | 0ed1a39a74a02bb66c8dee813e412977 | succeeded | true | 4 | 18 | 41892 / 1092 | USD 0.024241 | 20.028 | voluntary | false |
| S04 | 2 | E1 | 56c2dcfe50fec90e1f8a534e738c32c5 | succeeded | true | 16 | 39 | 314356 / 5784 | CNY 0.081227 | 40.286 | budget | false |
| S04 | 2 | E1 | cfb023d09d8e6e15b99ebb90e33973eb | succeeded | true | 4 | 15 | 36933 / 1040 | USD 0.022838 | 21.145 | voluntary | false |
| S04 | 2 | E3 | 023064f28c8912c6868668693e5524a1 | succeeded | true | 5 | 24 | 50698 / 1311 | USD 0.026347 | 23.924 | voluntary | false |
| S04 | 2 | E0 | c49cc175ec6b26bdec83aad0a3d7ecd2 | succeeded | false | 1 | 0 | 3607 / 318 | USD 0.006098 | 5.447 | voluntary | false |
| S04 | 2 | E2 | 2b7a43825c64aac8b684143c487d617b | succeeded | true | 4 | 16 | 39864 / 1016 | USD 0.025603 | 20.437 | voluntary | false |
| S04 | 3 | E1 | 8e1c3e8637ec567571d00ba10d0529a9 | succeeded | true | 16 | 28 | 252775 / 3636 | CNY 0.059598 | 29.902 | budget | false |
| S04 | 3 | E1 | 36089e0d5109edbdd3fb226c7fc49dee | succeeded | true | 4 | 18 | 38472 / 1192 | USD 0.023909 | 21.580 | voluntary | false |
| S04 | 3 | E3 | 1b879feb134d01aa80059366b2c1565e | succeeded | true | 4 | 16 | 34322 / 964 | USD 0.022159 | 17.524 | voluntary | false |
| S04 | 3 | E2 | 7693d2c3f629092bf86a4719b044ade4 | succeeded | true | 4 | 16 | 38647 / 983 | USD 0.024001 | 18.361 | voluntary | false |
| S04 | 3 | E0 | 5e12542c144b06fa870146374c75b8d8 | succeeded | false | 1 | 0 | 3607 / 306 | USD 0.006038 | 5.117 | voluntary | false |
| S05 | 1 | E1 | ccf74a935c488b82ae2c01f5ecf76cab | succeeded | true | 4 | 17 | 41179 / 1105 | USD 0.025496 | 22.224 | voluntary | false |
| S05 | 1 | E1 | 21c6092095421c67bbae3539794c21c0 | succeeded | true | 16 | 29 | 237925 / 4124 | CNY 0.061626 | 35.380 | budget | false |
| S05 | 1 | E0 | 2d0733b5012d8f37045e7ac4abdc6f41 | succeeded | false | 1 | 0 | 3607 / 230 | USD 0.005658 | 3.932 | voluntary | false |
| S05 | 1 | E3 | 1fc4163c7fd96b712ad031a1ddf72f98 | succeeded | true | 4 | 17 | 36167 / 1092 | USD 0.022844 | 21.255 | voluntary | false |
| S05 | 1 | E2 | 2d9f3d96b4b401e6edd3ce40303efce2 | succeeded | true | 5 | 24 | 67330 / 1390 | USD 0.034654 | 27.874 | voluntary | false |
| S05 | 2 | E3 | 2c6d17ede4910ca16178af5133fa3387 | succeeded | true | 5 | 24 | 55133 / 1429 | USD 0.029215 | 27.605 | voluntary | false |
| S05 | 2 | E2 | 9289ed5eb1c135e1e9e5fac600b4fb05 | succeeded | true | 4 | 18 | 42984 / 1198 | USD 0.028516 | 20.957 | voluntary | false |
| S05 | 2 | E1 | b88a09eaf817c96af6bed862d5f87a93 | succeeded | true | 4 | 19 | 40915 / 1231 | USD 0.026269 | 23.237 | voluntary | false |
| S05 | 2 | E0 | 462f16132e16efbc0f313b4a74160c7b | succeeded | false | 1 | 0 | 3607 / 312 | USD 0.006068 | 5.022 | voluntary | false |
| S05 | 2 | E1 | 3bc8740f04ce147d77e4383af38b21b1 | succeeded | true | 16 | 30 | 255373 / 5078 | CNY 0.066834 | 39.885 | budget | false |
| S05 | 3 | E3 | dfa860cd989d7edd0cf397f2e6e7295f | succeeded | true | 7 | 35 | 86775 / 1899 | USD 0.035808 | 35.271 | voluntary | false |
| S05 | 3 | E1 | c8523ef1acc21023f4069624b3f3c415 | succeeded | true | 14 | 22 | 192814 / 2796 | CNY 0.034533 | 24.606 | voluntary | false |
| S05 | 3 | E0 | 9352105ada0073da6a33ae05a5a358e1 | succeeded | false | 1 | 0 | 3607 / 243 | USD 0.005723 | 4.572 | voluntary | false |
| S05 | 3 | E2 | ccca7aacc352ee77f1f13cafc1264217 | succeeded | true | 5 | 24 | 58592 / 1469 | USD 0.031016 | 25.537 | voluntary | false |
| S05 | 3 | E1 | f090f19dbd0276f350772ea086048c0d | succeeded | true | 4 | 19 | 40949 / 1207 | USD 0.026000 | 22.410 | voluntary | false |
| S06 | 1 | E2 | 27fbeaad1109264f18c7e2c338c81c97 | succeeded | true | 3 | 10 | 23295 / 646 | USD 0.015490 | 22.809 | voluntary | false |
| S06 | 1 | E3 | 678caa56588f669d61a6b640cea9746e | succeeded | true | 3 | 10 | 20523 / 723 | USD 0.015406 | 21.411 | voluntary | false |
| S06 | 1 | E1 | 90947f5c4b370253e23fcfd441559903 | succeeded | true | 2 | 6 | 13760 / 592 | USD 0.013670 | 18.023 | voluntary | false |
| S06 | 1 | E1 | 42130e5c5ad9b88abccdeb5b47eb9494 | succeeded | true | 6 | 14 | 65722 / 2019 | CNY 0.023875 | 24.419 | voluntary | false |
| S06 | 1 | E0 | 0c4865e03f5fced88811ca29f6b9714c | succeeded | false | 1 | 0 | 3600 / 289 | USD 0.005945 | 4.721 | voluntary | false |
| S06 | 2 | E2 | c376306430f73971b985b41eb7225a43 | succeeded | true | 2 | 5 | 13966 / 522 | USD 0.013474 | 11.363 | voluntary | false |
| S06 | 2 | E3 | 9a5f46676ae9724cc30c81f79c8ebdb4 | succeeded | true | 2 | 6 | 13340 / 521 | USD 0.013116 | 18.006 | voluntary | false |
| S06 | 2 | E1 | 8a9ac0f5649447b697af47a05aa26394 | succeeded | true | 11 | 18 | 134433 / 4322 | CNY 0.035443 | 35.857 | voluntary | false |
| S06 | 2 | E1 | 9a4e3dcc6f6c1504d0184911f3dbd2c4 | succeeded | true | 2 | 6 | 13893 / 553 | USD 0.013641 | 18.011 | voluntary | false |
| S06 | 2 | E0 | bb8b74a4e61e1fbbb20417171eab8578 | succeeded | false | 1 | 0 | 3600 / 320 | USD 0.006100 | 5.445 | voluntary | false |
| S06 | 3 | E1 | 3e2d12c8202f48ffd2a10edafb8606e2 | succeeded | true | 3 | 10 | 23100 / 671 | USD 0.015719 | 21.729 | voluntary | false |
| S06 | 3 | E0 | 942ca525670280a4f5b1df1da60307d2 | succeeded | false | 1 | 0 | 3600 / 298 | USD 0.005990 | 4.916 | voluntary | false |
| S06 | 3 | E2 | 87602caf1c5918ece8160766e635b0d9 | succeeded | true | 2 | 6 | 13950 / 604 | USD 0.013864 | 18.327 | voluntary | false |
| S06 | 3 | E1 | c47ea88a6093263a234319be12233bac | succeeded | true | 10 | 19 | 130051 / 4438 | CNY 0.037421 | 34.102 | voluntary | false |
| S06 | 3 | E3 | 9bde666c25de10b0ce8e6ddd2d4f6e9b | succeeded | true | 2 | 5 | 13269 / 544 | USD 0.013142 | 9.915 | voluntary | false |
| S07 | 1 | E1 | 21fe4cc4333b73555c44661f9f58b2c4 | succeeded | true | 5 | 22 | 70548 / 1399 | USD 0.038068 | 26.424 | voluntary | false |
| S07 | 1 | E3 | 3c2d8f46a69f7aa08668132b284bc473 | succeeded | true | 6 | 26 | 78656 / 1520 | USD 0.036216 | 36.071 | voluntary | false |
| S07 | 1 | E1 | 699abe0894473eb72d96beb34618c14c | succeeded | true | 12 | 26 | 165978 / 3408 | CNY 0.036114 | 27.080 | voluntary | false |
| S07 | 1 | E0 | d7a4632360d869ff4e301992ffeb9bee | succeeded | false | 1 | 0 | 3665 / 304 | USD 0.006101 | 5.016 | voluntary | false |
| S07 | 1 | E2 | d70fdf893af793a8fbe9056b957e1808 | succeeded | true | 4 | 17 | 41576 / 1089 | USD 0.027171 | 19.674 | voluntary | false |
| S07 | 2 | E2 | 67ae919636e8ddff33253b3715c1c00b | succeeded | true | 4 | 16 | 44172 / 1119 | USD 0.029191 | 25.243 | voluntary | false |
| S07 | 2 | E1 | 4be6086acf7dbda220aa85869b811672 | succeeded | true | 4 | 17 | 44268 / 1015 | USD 0.029708 | 19.227 | voluntary | false |
| S07 | 2 | E0 | d714970721e37284e32d31993e1dc02c | succeeded | false | 1 | 0 | 3590 / 297 | USD 0.005972 | 6.445 | voluntary | false |
| S07 | 2 | E3 | c62ff474b2d4b3d59d9cdc67b67bb469 | succeeded | true | 5 | 21 | 64782 / 1233 | USD 0.035807 | 23.124 | voluntary | false |
| S07 | 2 | E1 | 1e3bf9dcc708ff8f1175debc4c1d9811 | succeeded | true | 16 | 28 | 220065 / 4000 | CNY 0.055062 | 29.170 | budget | false |
| S07 | 3 | E2 | afce538800abbd75ec589470f6719788 | succeeded | true | 4 | 17 | 43645 / 1026 | USD 0.026169 | 19.162 | voluntary | false |
| S07 | 3 | E1 | eca774f5a406c65014c4f42f99264e73 | succeeded | false | 16 | 29 | 241426 / 3768 | CNY 0.057933 | 29.614 | budget | false |
| S07 | 3 | E0 | 234af55700205c79e447dc01799b2cc3 | succeeded | false | 1 | 0 | 3660 / 300 | USD 0.006075 | 4.738 | voluntary | false |
| S07 | 3 | E3 | ce1daade11063cfd8c8842e8d41193f3 | succeeded | true | 7 | 30 | 104162 / 1668 | USD 0.041122 | 34.385 | voluntary | false |
| S07 | 3 | E1 | 7601845e79a4a0f7ef2aaf83a15ff847 | succeeded | true | 4 | 16 | 42855 / 1004 | USD 0.027006 | 18.590 | voluntary | false |

## 分层结果

每格为平均值 [最小, 最大] n=有效样本数。组件一致记 1，否则 0；机制正确/部分/错误记 1/0.5/0。基础设施失败和混合模型单列；评分失败不填零。三次样本很小，以下不是生产准确率。

| 场景 | 组 | 模型 | 组件一致 | 机制 | 证据覆盖 | 轮数 | 工具数 | 秒 | 医生单次账本 | 全部医生费用（含排除项） | 评委费用 | 可靠性 / 混合 / 未完成 |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| S01 | E0 | gpt-6.1-terra | 0.000 [0.000, 0.000] n=3 | 0.000 [0.000, 0.000] n=2 | 0.200 [0.200, 0.200] n=2 | 1.000 [1.000, 1.000] n=3 | 0.000 [0.000, 0.000] n=3 | 5.501 [5.142, 6.170] n=3 | USD 0.006 [0.006, 0.006] n=3 | USD 0.018886 | CNY 0.008468 | 0 / 0 / 0 |
| S01 | E1 | deepseek-flash | 1.000 [1.000, 1.000] n=3 | 1.000 [1.000, 1.000] n=3 | 0.800 [0.800, 0.800] n=3 | 7.667 [6.000, 10.000] n=3 | 19.000 [18.000, 20.000] n=3 | 24.129 [19.531, 31.058] n=3 | CNY 0.035 [0.029, 0.045] n=3 | CNY 0.105048 | CNY 0.054161 | 0 / 0 / 0 |
| S01 | E1 | gpt-6.1-terra | 1.000 [1.000, 1.000] n=3 | 1.000 [1.000, 1.000] n=3 | 0.800 [0.800, 0.800] n=3 | 4.333 [4.000, 5.000] n=3 | 15.667 [14.000, 17.000] n=3 | 23.274 [20.622, 26.168] n=3 | USD 0.026 [0.024, 0.027] n=3 | USD 0.077677 | CNY 0.042140 | 0 / 0 / 0 |
| S01 | E2 | gpt-6.1-terra | 1.000 [1.000, 1.000] n=3 | 1.000 [1.000, 1.000] n=3 | 0.800 [0.800, 0.800] n=3 | 5.000 [5.000, 5.000] n=3 | 17.333 [15.000, 21.000] n=3 | 23.845 [21.489, 27.649] n=3 | USD 0.026 [0.022, 0.031] n=3 | USD 0.076539 | CNY 0.039987 | 0 / 0 / 0 |
| S01 | E3 | gpt-6.1-terra | 1.000 [1.000, 1.000] n=3 | 1.000 [1.000, 1.000] n=3 | 0.733 [0.600, 0.800] n=3 | 4.333 [4.000, 5.000] n=3 | 16.000 [15.000, 18.000] n=3 | 23.363 [20.828, 26.735] n=3 | USD 0.024 [0.021, 0.028] n=3 | USD 0.072768 | CNY 0.039024 | 0 / 0 / 0 |
| S02 | E0 | gpt-6.1-terra | 0.000 [0.000, 0.000] n=3 | 0.000 [0.000, 0.000] n=3 | 0.250 [0.250, 0.250] n=3 | 1.000 [1.000, 1.000] n=3 | 0.000 [0.000, 0.000] n=3 | 5.938 [5.000, 7.170] n=3 | USD 0.006 [0.006, 0.006] n=3 | USD 0.018803 | CNY 0.008280 | 0 / 0 / 0 |
| S02 | E1 | deepseek-flash | 0.667 [0.000, 1.000] n=3 | 0.833 [0.500, 1.000] n=3 | 0.750 [0.500, 1.000] n=3 | 13.667 [9.000, 16.000] n=3 | 24.667 [18.000, 31.000] n=3 | 30.249 [22.201, 37.977] n=3 | CNY 0.049 [0.029, 0.068] n=3 | CNY 0.147964 | CNY 0.055550 | 0 / 0 / 0 |
| S02 | E1 | gpt-6.1-terra | 1.000 [1.000, 1.000] n=3 | 1.000 [1.000, 1.000] n=3 | 1.000 [1.000, 1.000] n=3 | 4.667 [4.000, 6.000] n=3 | 16.667 [12.000, 22.000] n=3 | 22.376 [19.271, 25.694] n=3 | USD 0.023 [0.019, 0.027] n=3 | USD 0.068424 | CNY 0.033600 | 0 / 0 / 0 |
| S02 | E2 | gpt-6.1-terra | 1.000 [1.000, 1.000] n=3 | 1.000 [1.000, 1.000] n=3 | 0.833 [0.750, 1.000] n=3 | 4.333 [4.000, 5.000] n=3 | 16.000 [14.000, 20.000] n=3 | 23.167 [21.100, 26.586] n=3 | USD 0.025 [0.021, 0.030] n=3 | USD 0.074232 | CNY 0.037727 | 0 / 0 / 0 |
| S02 | E3 | gpt-6.1-terra | 1.000 [1.000, 1.000] n=3 | 1.000 [1.000, 1.000] n=3 | 1.000 [1.000, 1.000] n=3 | 4.333 [4.000, 5.000] n=3 | 16.000 [13.000, 21.000] n=3 | 23.626 [20.551, 26.763] n=3 | USD 0.024 [0.022, 0.028] n=3 | USD 0.122733 | CNY 0.038567 | 1 / 0 / 0 |
| S03 | E0 | gpt-6.1-terra | 0.000 [0.000, 0.000] n=3 | 0.000 [0.000, 0.000] n=3 | 0.000 [0.000, 0.000] n=3 | 1.000 [1.000, 1.000] n=3 | 0.000 [0.000, 0.000] n=3 | 4.970 [4.663, 5.218] n=3 | USD 0.006 [0.006, 0.006] n=3 | USD 0.018019 | CNY 0.007346 | 0 / 0 / 0 |
| S03 | E1 | deepseek-flash | 1.000 [1.000, 1.000] n=3 | 1.000 [1.000, 1.000] n=3 | 0.583 [0.000, 1.000] n=3 | 14.667 [13.000, 17.000] n=3 | 28.000 [23.000, 31.000] n=3 | 37.788 [27.405, 45.514] n=3 | CNY 0.055 [0.037, 0.068] n=3 | CNY 0.165126 | CNY 0.060282 | 0 / 0 / 0 |
| S03 | E1 | gpt-6.1-terra | 1.000 [1.000, 1.000] n=3 | 1.000 [1.000, 1.000] n=3 | 0.917 [0.750, 1.000] n=3 | 4.333 [4.000, 5.000] n=3 | 15.667 [14.000, 19.000] n=3 | 19.669 [18.013, 22.544] n=3 | USD 0.023 [0.023, 0.025] n=3 | USD 0.070419 | CNY 0.037163 | 0 / 0 / 0 |
| S03 | E2 | gpt-6.1-terra | 1.000 [1.000, 1.000] n=3 | 1.000 [1.000, 1.000] n=3 | 0.833 [0.750, 1.000] n=3 | 4.667 [4.000, 5.000] n=3 | 17.000 [12.000, 20.000] n=3 | 22.895 [18.323, 25.806] n=3 | USD 0.024 [0.022, 0.026] n=3 | USD 0.071930 | CNY 0.036804 | 0 / 0 / 0 |
| S03 | E3 | gpt-6.1-terra | 1.000 [1.000, 1.000] n=3 | 1.000 [1.000, 1.000] n=2 | 0.750 [0.750, 0.750] n=2 | 4.667 [4.000, 5.000] n=3 | 18.333 [15.000, 21.000] n=3 | 23.203 [20.244, 25.400] n=3 | USD 0.027 [0.023, 0.029] n=3 | USD 0.079854 | CNY 0.043137 | 0 / 0 / 0 |
| S04 | E0 | gpt-6.1-terra | 0.000 [0.000, 0.000] n=3 | 0.000 [0.000, 0.000] n=3 | 0.000 [0.000, 0.000] n=3 | 1.000 [1.000, 1.000] n=3 | 0.000 [0.000, 0.000] n=3 | 5.074 [4.658, 5.447] n=3 | USD 0.006 [0.006, 0.006] n=3 | USD 0.017844 | CNY 0.009093 | 0 / 0 / 0 |
| S04 | E1 | deepseek-flash | 1.000 [1.000, 1.000] n=3 | 0.833 [0.500, 1.000] n=3 | 0.333 [0.167, 0.500] n=3 | 16.333 [16.000, 17.000] n=3 | 33.333 [28.000, 39.000] n=3 | 40.299 [29.902, 50.709] n=3 | CNY 0.076 [0.060, 0.087] n=3 | CNY 0.227449 | CNY 0.074531 | 0 / 0 / 0 |
| S04 | E1 | gpt-6.1-terra | 1.000 [1.000, 1.000] n=3 | 1.000 [1.000, 1.000] n=3 | 0.556 [0.500, 0.667] n=3 | 4.000 [4.000, 4.000] n=3 | 17.000 [15.000, 18.000] n=3 | 20.918 [20.028, 21.580] n=3 | USD 0.024 [0.023, 0.024] n=3 | USD 0.070988 | CNY 0.039590 | 0 / 0 / 0 |
| S04 | E2 | gpt-6.1-terra | 1.000 [1.000, 1.000] n=3 | 1.000 [1.000, 1.000] n=3 | 0.611 [0.500, 0.667] n=3 | 4.000 [4.000, 4.000] n=3 | 16.333 [16.000, 17.000] n=3 | 19.565 [18.361, 20.437] n=3 | USD 0.025 [0.024, 0.027] n=3 | USD 0.076247 | CNY 0.044814 | 0 / 0 / 0 |
| S04 | E3 | gpt-6.1-terra | 1.000 [1.000, 1.000] n=3 | 0.833 [0.500, 1.000] n=3 | 0.500 [0.333, 0.667] n=3 | 4.333 [4.000, 5.000] n=3 | 19.667 [16.000, 24.000] n=3 | 21.187 [17.524, 23.924] n=3 | USD 0.024 [0.022, 0.026] n=3 | USD 0.072556 | CNY 0.040988 | 0 / 0 / 0 |
| S05 | E0 | gpt-6.1-terra | 0.000 [0.000, 0.000] n=3 | 0.000 [0.000, 0.000] n=3 | 0.000 [0.000, 0.000] n=3 | 1.000 [1.000, 1.000] n=3 | 0.000 [0.000, 0.000] n=3 | 4.509 [3.932, 5.022] n=3 | USD 0.006 [0.006, 0.006] n=3 | USD 0.017449 | CNY 0.008259 | 0 / 0 / 0 |
| S05 | E1 | deepseek-flash | 1.000 [1.000, 1.000] n=3 | 0.500 [0.000, 1.000] n=3 | 0.200 [0.000, 0.400] n=3 | 15.333 [14.000, 16.000] n=3 | 27.000 [22.000, 30.000] n=3 | 33.290 [24.606, 39.885] n=3 | CNY 0.054 [0.035, 0.067] n=3 | CNY 0.162993 | CNY 0.062120 | 0 / 0 / 0 |
| S05 | E1 | gpt-6.1-terra | 1.000 [1.000, 1.000] n=3 | 0.500 [0.500, 0.500] n=1 | 0.200 [0.200, 0.200] n=1 | 4.000 [4.000, 4.000] n=3 | 18.333 [17.000, 19.000] n=3 | 22.624 [22.224, 23.237] n=3 | USD 0.026 [0.025, 0.026] n=3 | USD 0.077765 | CNY 0.102858 | 0 / 0 / 0 |
| S05 | E2 | gpt-6.1-terra | 1.000 [1.000, 1.000] n=3 | 0.500 [0.500, 0.500] n=2 | 0.300 [0.200, 0.400] n=2 | 4.667 [4.000, 5.000] n=3 | 22.000 [18.000, 24.000] n=3 | 24.789 [20.957, 27.874] n=3 | USD 0.031 [0.029, 0.035] n=3 | USD 0.094186 | CNY 0.053626 | 0 / 0 / 0 |
| S05 | E3 | gpt-6.1-terra | 1.000 [1.000, 1.000] n=3 | 0.750 [0.500, 1.000] n=2 | 0.300 [0.200, 0.400] n=2 | 5.333 [4.000, 7.000] n=3 | 25.333 [17.000, 35.000] n=3 | 28.044 [21.255, 35.271] n=3 | USD 0.029 [0.023, 0.036] n=3 | USD 0.087867 | CNY 0.049055 | 0 / 0 / 0 |
| S06 | E0 | gpt-6.1-terra | 0.000 [0.000, 0.000] n=3 | 0.000 [0.000, 0.000] n=3 | 0.000 [0.000, 0.000] n=3 | 1.000 [1.000, 1.000] n=3 | 0.000 [0.000, 0.000] n=3 | 5.027 [4.721, 5.445] n=3 | USD 0.006 [0.006, 0.006] n=3 | USD 0.018035 | CNY 0.007579 | 0 / 0 / 0 |
| S06 | E1 | deepseek-flash | 1.000 [1.000, 1.000] n=3 | 1.000 [1.000, 1.000] n=3 | 0.750 [0.750, 0.750] n=3 | 9.000 [6.000, 11.000] n=3 | 17.000 [14.000, 19.000] n=3 | 31.459 [24.419, 35.857] n=3 | CNY 0.032 [0.024, 0.037] n=3 | CNY 0.096739 | CNY 0.042904 | 0 / 0 / 0 |
| S06 | E1 | gpt-6.1-terra | 1.000 [1.000, 1.000] n=3 | 1.000 [1.000, 1.000] n=3 | 0.500 [0.250, 0.750] n=3 | 2.333 [2.000, 3.000] n=3 | 7.333 [6.000, 10.000] n=3 | 19.254 [18.011, 21.729] n=3 | USD 0.014 [0.014, 0.016] n=3 | USD 0.043030 | CNY 0.019956 | 0 / 0 / 0 |
| S06 | E2 | gpt-6.1-terra | 1.000 [1.000, 1.000] n=3 | 1.000 [1.000, 1.000] n=3 | 0.417 [0.250, 0.500] n=3 | 2.333 [2.000, 3.000] n=3 | 7.000 [5.000, 10.000] n=3 | 17.500 [11.363, 22.809] n=3 | USD 0.014 [0.013, 0.015] n=3 | USD 0.042828 | CNY 0.019267 | 0 / 0 / 0 |
| S06 | E3 | gpt-6.1-terra | 1.000 [1.000, 1.000] n=3 | 1.000 [1.000, 1.000] n=3 | 0.583 [0.250, 0.750] n=3 | 2.333 [2.000, 3.000] n=3 | 7.000 [5.000, 10.000] n=3 | 16.444 [9.915, 21.411] n=3 | USD 0.014 [0.013, 0.015] n=3 | USD 0.041664 | CNY 0.019727 | 0 / 0 / 0 |
| S07 | E0 | gpt-6.1-terra | 0.000 [0.000, 0.000] n=3 | 0.000 [0.000, 0.000] n=3 | 0.000 [0.000, 0.000] n=3 | 1.000 [1.000, 1.000] n=3 | 0.000 [0.000, 0.000] n=3 | 5.400 [4.738, 6.445] n=3 | USD 0.006 [0.006, 0.006] n=3 | USD 0.018148 | CNY 0.007884 | 0 / 0 / 0 |
| S07 | E1 | deepseek-flash | 0.667 [0.000, 1.000] n=3 | 0.500 [0.000, 1.000] n=2 | 0.250 [0.000, 0.500] n=2 | 14.667 [12.000, 16.000] n=3 | 27.667 [26.000, 29.000] n=3 | 28.621 [27.080, 29.614] n=3 | CNY 0.050 [0.036, 0.058] n=3 | CNY 0.149109 | CNY 0.056880 | 0 / 0 / 0 |
| S07 | E1 | gpt-6.1-terra | 1.000 [1.000, 1.000] n=3 | 0.833 [0.500, 1.000] n=3 | 0.583 [0.500, 0.750] n=3 | 4.333 [4.000, 5.000] n=3 | 18.333 [16.000, 22.000] n=3 | 21.414 [18.590, 26.424] n=3 | USD 0.032 [0.027, 0.038] n=3 | USD 0.094782 | CNY 0.056162 | 0 / 0 / 0 |
| S07 | E2 | gpt-6.1-terra | 1.000 [1.000, 1.000] n=3 | 1.000 [1.000, 1.000] n=3 | 0.833 [0.750, 1.000] n=3 | 4.000 [4.000, 4.000] n=3 | 16.667 [16.000, 17.000] n=3 | 21.360 [19.162, 25.243] n=3 | USD 0.028 [0.026, 0.029] n=3 | USD 0.082531 | CNY 0.045502 | 0 / 0 / 0 |
| S07 | E3 | gpt-6.1-terra | 1.000 [1.000, 1.000] n=3 | 1.000 [1.000, 1.000] n=3 | 0.833 [0.750, 1.000] n=3 | 6.000 [5.000, 7.000] n=3 | 25.667 [21.000, 30.000] n=3 | 31.193 [23.124, 36.071] n=3 | USD 0.038 [0.036, 0.041] n=3 | USD 0.113145 | CNY 0.065654 | 0 / 0 / 0 |

## 评分与失败明细

以下是原始结论与评委理由；评委也是模型，人工一致率未填写前不能视为标准答案。格式、预算、可靠性和评分失败分别显示。推理错误、工具不足、数据缺失以及评分有误的最终分类，需要结合原步骤逐条核对。

| 槽位 | 医生错误 / 预算原因 | 机制 / 评分状态 | 评分理由 | 证据覆盖 | 评委账本 | S12 实际暴露 | 写提议 / 执行 |
|---|---|---|---|---|---|---|---|
| phase08-formal-20261029/S01/1/primary-E0 |   | incorrect | 报告仅复述告警（RPC P99、连接池等待、行锁等待），未定位根因，明确将组件与机制记为 unknown，也未提出缺失索引这一参考答案机制，属于症状复述。 | 0.200 | CNY 0.002873 | false | 0 / 0 |
| phase08-formal-20261029/S01/1/comparison-E1 |   | correct | 报告指出stock表sku_id唯一索引uk_stock_sku被删除，表上只剩PRIMARY(id)，按sku_id的查询与扣减UPDATE退化为全表扫描（步骤17/31 EXPLAIN type=ALL约49.9万行），更新锁住大量行导致行锁等待链（步骤7/8）与Deduct超时、连接池等待，主因方向与参考答案一致，无相反结论。 | 0.800 | CNY 0.022325 | false | 1 / 0 |
| phase08-formal-20261029/S01/1/primary-E1 |   | correct | 报告正确指出 shop.stock 缺少 sku_id 索引，扣减/查询需全表扫描（约 49.9 万行），UPDATE 持锁导致行锁等待与连接池排队，进而造成库存 RPC 延迟和下单超时。与参考答案的 missing_index 机制一致，无相反结论。索引恢复未执行，报告未编造恢复证据。 | 0.800 | CNY 0.012571 | false | 1 / 0 |
| phase08-formal-20261029/S01/1/primary-E2 |   | correct | 报告指出 stock 表缺失 sku_id 索引导致按 sku_id 查询和扣减全表扫描约 49.9 万行，慢更新持锁并可能扩大行锁竞争和连接池排队，进而使库存 RPC 超时影响下单；与参考答案因果关系一致。锁阻塞链未捕获被明确列为未知，未做相反结论。 | 0.800 | CNY 0.010987 | false | 1 / 0 |
| phase08-formal-20261029/S01/1/primary-E3 |   | correct | 报告定位 shop.stock 缺少 sku_id 唯一索引，按 sku_id 查找退化为全表扫描且持锁导致行锁争用、连接池排队与库存 RPC 超时，进而影响下单；与参考答案因果链一致，无相反结论。 | 0.800 | CNY 0.015734 | false | 1 / 0 |
| phase08-formal-20261029/S01/2/primary-E2 |   | correct | 报告将根因定位为 shop.stock 缺少 sku_id 唯一索引，导致按 sku_id 的查询/扣减全表扫描并持锁等待，进而库存 RPC 超时，与参考答案机制一致；有步骤17索引缺失、步骤26全表扫描、步骤12主键记录锁等待、步骤20 UPDATE 耗时约717ms 等证据支撑，无相反结论。 | 0.800 | CNY 0.016569 | false | 1 / 0 |
| phase08-formal-20261029/S01/2/comparison-E1 |   | correct | 报告准确指出 shop.stock 丢失 sku_id 唯一索引导致按 sku_id 查询全表扫描，拉长持锁时间，引发行锁等待、连接池排队和库存 RPC P99 超时，与参考机制 missing_index 一致，无相反结论。 | 0.800 | CNY 0.015425 | false | 1 / 0 |
| phase08-formal-20261029/S01/2/primary-E3 |   | correct | 报告指出 stock 缺少 sku_id 索引导致全表扫描，进而延长锁持有、引发行锁等待与连接池等待，最终库存 RPC 超时；与参考答案的 missing_index 机制一致，未排除主因。 | 0.600 | CNY 0.012113 | false | 1 / 0 |
| phase08-formal-20261029/S01/2/primary-E1 |   | correct | 报告主因为 stock 表缺少 sku_id 索引，导致按 sku_id 定位需全表扫描、更新持锁范围扩大、行锁等待与扣减超时，并间接造成连接池等待与 RPC 延迟，与参考答案一致，无相反结论。 | 0.800 | CNY 0.015296 | false | 1 / 0 |
| phase08-formal-20261029/S01/2/primary-E0 |   | incorrect | 报告仅复述库存 RPC P99 高和数据库连接池等待告警，明确将根因列为 unknown，未识别扣减语句缺少 sku_id 唯一索引导致行扫描、锁持有与等待的机制，属仅症状复述且未给出主因。 | 0.200 | CNY 0.002742 | false | 0 / 0 |
| phase08-formal-20261029/S01/3/primary-E1 |   | correct | 报告主因与参考一致：stock 缺 sku_id 索引导致按 sku_id 扣减全表扫描（约 49.9 万行），扫描持锁引发行锁等待与连接占用，进而造成库存 RPC 延迟/超时。证据链完整（告警、执行计划 ALL、慢扣减 SQL、goroutine 池排队、行锁指标），并恰当标注未证实的部分。 | 0.800 | CNY 0.014273 | false | 1 / 0 |
| phase08-formal-20261029/S01/3/primary-E2 |   | correct | 报告确认 stock 表缺少 sku_id 索引，导致按 sku_id 查询/扣减需全表扫描（约 49.9 万行），SQL 耗时和锁竞争上升，连接被长占引起连接池排队与库存 RPC 超时，与参考答案的机制一致，无相反结论。  里程碑1：告警与调用链显示库存 RPC P99 约 0.91 秒、Deduct 约 800ms 超时（DEADLINE_EXCEEDED），下单变慢/超时已被观察到（步骤6、14）。 里程碑2：步骤14 调用链中 stock 的 UPDATE span 约 510/784 ms，占 Deduct 800ms 的绝大部分，满足“库存 UPDATE 占用主要时间”。 里程碑3：步骤17 行锁等待指标在故障期上升（最后值 14834，最大 24727），但仍未执行恢复索引，缺少“恢复后下降”这一半，故未达成。 | 0.800 | CNY 0.012431 | false | 1 / 0 |
| phase08-formal-20261029/S01/3/primary-E0 |   | 评分失败 | 里程碑缺失、重复或覆盖规则项 | — | CNY 0.002853 | false | 0 / 0 |
| phase08-formal-20261029/S01/3/primary-E3 |   | correct | 报告主因与参考答案一致：stock 表缺少 sku_id 索引，扣减按 sku_id 定位时全表扫描约 49.8 万行，更新持锁使其他扣减等待，造成库存 RPC 延迟、连接池排队并波及下单。已用步骤13的 EXPLAIN ALL 全表扫描、步骤15索引缺失、步骤10行锁等待、步骤14 Deduct 等待 SQL、步骤19 RPC 超时支撑，无相反结论。 | 0.800 | CNY 0.011177 | false | 1 / 0 |
| phase08-formal-20261029/S01/3/comparison-E1 |   | correct | 报告定位到 mysql:shop.stock 缺失 sku_id 唯一索引，导致按 sku_id 查询全表扫描并放大行锁等待，进而拖慢库存 RPC、占满连接池、抬高 order-service 错误率，与参考答案的 missing_index 因果关系一致；证据由索引清单、EXPLAIN、锁指标和慢 trace 支撑，无相反结论。 | 0.800 | CNY 0.016411 | false | 0 / 0 |
| phase08-formal-20261029/S02/1/comparison-E1 |  轮数达到上限 | partial | 报告正确识别了慢点在 GET /orders 的按用户分页查询并占满连接池，但参考答案的主因是 orders 表缺索引导致扫描和排序；报告只把索引缺失列为“推断（未验证）”，同时把主因归结为连接池饱和，EXPLAIN/索引均未取得。因此只找到部分原因，关键机制未证实，且主因与参考答案不完全一致。 | 0.500 | CNY 0.023796 | false | 0 / 0 |
| phase08-formal-20261029/S02/1/primary-E0 |   | incorrect | 报告仅依据初始告警指出订单列表 P99 延迟与数据库连接池等待，明确将根因判为 unknown，未识别联合索引消失导致 orders 查询扫描与排序大量历史订单这一主因，也未提出任何缺失索引相关机制，属于只有症状复述且未确定主因。 | 0.250 | CNY 0.002988 | false | 0 / 0 |
| phase08-formal-20261029/S02/1/primary-E3 |   | correct | 报告主因与参考答案一致：mysql:shop.orders 缺少列表查询所需的 (user_id, created_at) 联合索引，导致全表扫描加 filesort、扫描约552万行，订单列表超时；并指出慢查询占用共享连接可能引发连接池排队。组件与机制均正确。 | 1.000 | CNY 0.012063 | false | 1 / 0 |
| phase08-formal-20261029/S02/1/primary-E2 |   | correct | 报告定位到 orders 列表 SQL 因缺索引导致全表扫描与 filesort，扫描约 552 万行，进而占满订单服务连接池、引发超时，与参考答案的 missing_index 机制一致，且有步骤 6、25、23、16 等证据支撑。 | 0.750 | CNY 0.016476 | false | 1 / 0 |
| phase08-formal-20261029/S02/1/primary-E1 |   | correct | 报告将主因定位为 mysql:shop.orders 缺少按 (user_id, created_at) 的分页复合索引，导致列表查询全表扫描约 552 万行并使用 filesort，慢查询占满连接池放大延迟；与参考答案的 missing_index 机制一致，且无相反结论。跨接口影响程度也标为待验证，未武断。 | 1.000 | CNY 0.009152 | false | 1 / 0 |
| phase08-formal-20261029/S02/2/comparison-E1 |   | correct | 报告主因与参考答案一致：GET /orders 的按用户过滤+创建时间排序查询在 orders 表缺少匹配索引，导致全表扫描加 filesort，慢查询占用连接池引发告警。步骤 24 区分了 /orders 与 /products/:id，步骤 27 定位到 orders 查询，步骤 33 EXPLAIN 显示 rows≈5.5M 与 Using filesort，均有真实观测支持。 | 1.000 | CNY 0.014112 | false | 1 / 0 |
| phase08-formal-20261029/S02/2/primary-E1 |   | correct | 报告主因与参考答案一致：orders 缺少支持 user_id 过滤与 created_at 排序的联合索引，导致全表扫描加 filesort，引发慢查询与连接等待；并正确给出组件 mysql:shop.orders。附带连接数接近上限仅作为加剧因素且标注来源待核实，不构成相反主因。 | 1.000 | CNY 0.011355 | false | 0 / 0 |
| phase08-formal-20261029/S02/2/primary-E0 |   | incorrect | 报告仅复述订单列表延迟与数据库连接池等待告警，明确表示无法确定根因，未提出丢失索引/扫描排序大量历史订单的机制，属于只有症状复述与假设性推测。 | 0.250 | CNY 0.002554 | false | 0 / 0 |
| phase08-formal-20261029/S02/2/primary-E2 |   | correct | 报告主因为 orders 缺少 (user_id, created_at) 联合索引，导致列表查询全表扫描并 filesort，占用资源与连接，与参考答案 missing_index 机制一致；未提出相反结论。 | 1.000 | CNY 0.010188 | false | 1 / 0 |
| phase08-formal-20261029/S02/2/primary-E3 | 用户取消或服务停止  | 未评分 |  | — | CNY 0.000000 | false | 0 / 0 |
| phase08-formal-20261029/S02/2/primary-E3/makeup |   | correct | 报告主因明确为 orders 缺少 (user_id, created_at) 联合索引，导致列表分页查询全表扫描并 filesort、列表协程长时间占用连接池，与参考答案一致，且未提出相冲突主因。 | 1.000 | CNY 0.011699 | false | 1 / 0 |
| phase08-formal-20261029/S02/3/comparison-E1 |  轮数达到上限 | correct | 报告将根因定位为 orders 表缺失 (user_id, created_at) 联合索引，导致按用户分页查询全表扫描并额外排序，进而拖慢连接池与接口延迟；与参考答案的 missing_index 机制一致，且未提出相反主因。 | 0.750 | CNY 0.017642 | false | 0 / 0 |
| phase08-formal-20261029/S02/3/primary-E3 |   | correct | 报告定位为 orders 缺少 (user_id, created_at) 联合索引，导致订单列表查询全表扫描并 filesort，长时间占用连接池，进而拖慢下单；与参考的 missing_index 机制一致，无相反结论。虽然 EXPLAIN 并非取自实际 SQL 文本，但机制判断方向正确。 | 1.000 | CNY 0.014805 | false | 1 / 0 |
| phase08-formal-20261029/S02/3/primary-E2 |   | correct | 报告指出订单列表查询缺少 user_id、created_at 联合索引，导致大表扫描/排序，长时间占用连接引发连接池等待与超时，与参考答案的缺失索引机制一致；虽 EXPLAIN 未成功，但明确将扫描/排序作为高度怀疑而非已证事实，没有相反结论。 | 0.750 | CNY 0.011063 | false | 1 / 0 |
| phase08-formal-20261029/S02/3/primary-E0 |   | incorrect | 报告只描述了订单列表 P99 与数据库连接等待两个告警，未确定根因，组件为 unknown，也没有指出 orders 查询缺少索引导致扫描与排序的机制。虽然承认可能存在慢查询，但明确表示不能确认因果关系，主因缺失，故机制判为 incorrect。 | 0.250 | CNY 0.002738 | false | 0 / 0 |
| phase08-formal-20261029/S02/3/primary-E1 |   | correct | 报告正确识别慢在订单列表的 GET /orders：步骤7/28显示 GET /orders 的 SQL 耗时 2.948s；步骤25 EXPLAIN 为 ALL、扫描约553万行并 Using filesort；步骤31确认 orders 仅有 PRIMARY(id)，缺少 user_id、created_at 联合索引，导致全表扫描+排序并造成连接池压力。机制为 missing_index，与参考答案一致，无相反结论。 | 1.000 | CNY 0.013093 | false | 1 / 0 |
| phase08-formal-20261029/S03/1/primary-E3 |   | 评分失败 | 评分 JSON 格式不合法 | — | CNY 0.015312 | false | 1 / 0 |
| phase08-formal-20261029/S03/1/comparison-E1 |   | correct | 报告主因与参考答案一致：inventory-service 重启后连接池上限由 20 降为 2，并发请求超出后排队借连接，触发 DatabasePoolWaiting；并用 MySQL 连接数、无锁等待、stock 索引正常排除了数据库侧原因，未见相反结论。 | 1.000 | CNY 0.019659 | false | 0 / 0 |
| phase08-formal-20261029/S03/1/primary-E2 |   | correct | 报告依据步骤23/25的指标把连接池上限由20降为2定位为主因，并用步骤14的满载（2条均用、空闲0）与步骤24的等待速率解释请求排队借连接、SQL时间被压缩，与参考机制一致；步骤22的等待时长数据只是补充说明，未推翻主因。 | 1.000 | CNY 0.012977 | false | 1 / 0 |
| phase08-formal-20261029/S03/1/primary-E1 |   | correct | 报告以连接池上限由20降至2、并发请求排队借连接为根因，符合参考机制 pool_capacity；证据来自真实指标步骤（14、24、22等），未提出相反结论。虽未取得启动日志直接打印上限变化，但 step 14/26 显示观测窗口内 max_open_connections 从20降至2，关联合理。 | 1.000 | CNY 0.013113 | false | 1 / 0 |
| phase08-formal-20261029/S03/1/primary-E0 |   | incorrect | 报告只根据初始告警描述连接等待率，未做任何实际查询，根因结论为 unknown，并明确未确认是否为连接池容量问题；未解释并发请求超过两条连接导致排队、SQL 时间被压缩这一机制。 | 0.000 | CNY 0.002424 | false | 0 / 0 |
| phase08-formal-20261029/S03/2/comparison-E1 |   | correct | 报告主因定位在 inventory-service:database_pool 的连接池上限被压到 2（其余服务为 20），并发请求排队借连接导致 DatabasePoolWaiting；并用步骤 30/40 的上限对比、步骤 29 的等待速率、步骤 12 的 MySQL 未饱和与步骤 22 的索引正常做了排除，符合参考答案的 pool_capacity 机制，无相反结论。轻微瑕疵是把反复重启当作并列诱因，但核心因果链完整。 | 0.750 | CNY 0.018833 | false | 0 / 0 |
| phase08-formal-20261029/S03/2/primary-E1 |   | correct | 报告主因为库存服务数据库连接池上限由20降至2，并发请求排队等连接，等待计数与等待时长累积，与参考机制一致且无相反结论。 | 0.750 | CNY 0.011952 | false | 1 / 0 |
| phase08-formal-20261029/S03/2/primary-E0 |   | incorrect | 报告仅复述连接池等待告警，明确表示根因无法确定，未指出连接池容量不足（上限被改为 2）这一关键因果关系，属于只有症状复述。 | 0.000 | CNY 0.002470 | false | 0 / 0 |
| phase08-formal-20261029/S03/2/primary-E2 |   | correct | 报告主因明确为库存服务重启切换版本后连接池上限从20降至2，并发请求争用两条连接导致排队等待，与参考机制的 pool_capacity 因果一致，且未给出相反结论。 | 0.750 | CNY 0.011607 | false | 1 / 0 |
| phase08-formal-20261029/S03/2/primary-E3 |   | correct | 报告认定连接池上限降至2（范围内max=20、last=2），连接占满（in_use=2、idle=0）导致并发请求排队借连接，等待速率约35.5次/秒，机制与参考答案的 pool_capacity 一致，无相反结论。 | 0.750 | CNY 0.011824 | false | 1 / 0 |
| phase08-formal-20261029/S03/3/primary-E2 |   | correct | 报告将主因定位为库存服务数据库连接池上限降至2导致连接排队等待，与参考答案 pool_capacity 机制一致，并附有多项观测证据支撑。 | 0.750 | CNY 0.012220 | false | 1 / 0 |
| phase08-formal-20261029/S03/3/primary-E0 |   | incorrect | 报告只复述连接池等待告警这一症状，未确认连接池容量被改小的机制，主因标注 unknown，未给出参考答案的关键因果链，也未做任何实际排查步骤。 | 0.000 | CNY 0.002452 | false | 0 / 0 |
| phase08-formal-20261029/S03/3/primary-E1 |   | correct | 报告定位库存服务连接池上限由20降至2，并发请求争用少量连接导致排队等待、等待率告警，与参考答案的 pool_capacity 机制一致；证据（步骤15指标、步骤10堆栈、步骤5/7排除锁等待）充分，无相反结论。 | 1.000 | CNY 0.012098 | false | 1 / 0 |
| phase08-formal-20261029/S03/3/primary-E3 |   | correct | 报告主因与参考答案一致：库存服务连接池上限从20降至2，两条连接占满后请求排队借连接，触发 DatabasePoolWaiting 告警。证据链完整（step26偏移查询得20、step22/16 当前为2、step21 等待速率、step23 平均等待时长），并明确指出上限下降的具体配置/代码成因尚未核实，未排除主因，也无相反结论。 | 0.750 | CNY 0.016001 | false | 1 / 0 |
| phase08-formal-20261029/S03/3/comparison-E1 |  轮数达到上限 | correct | 报告把主因定位为 inventory-service 数据库连接池上限被压到 2（观测期内 2～20 波动），而请求约 180/s，导致 sql.DB 层借连接排队、go_sql_wait 长期 32～37/s，与参考答案 pool_capacity 的因果关系一致；同时用 MySQL 无行锁等待、连接未打满排除了数据库端瓶颈，未提出相反主因。 | 0.000 | CNY 0.021790 | false | 0 / 0 |
| phase08-formal-20261029/S04/1/comparison-E1 |  轮数达到上限 | correct | 报告主因是 order-service 商品缓存在新版本关闭 cache_singleflight，导致同一商品键的并发查询未合并、各自回源，回源比例升高触发 CacheLoadsHigh；与参考答案的 concurrent_cache_miss 机制一致，且未提出冲突主因（Redis 淘汰/过期、行锁等待均被排除）。 | 0.333 | CNY 0.026591 | false | 0 / 0 |
| phase08-formal-20261029/S04/1/primary-E0 |   | incorrect | 报告仅复述了 CacheLoadsHigh 告警值约74.8%和告警时间，明确表示缺少命中、流量与后端证据，组件与机制均为 unknown，未提出 singleflight 未合并同键并发查询导致重复回源这一主因，属于只有症状复述。 | 0.000 | CNY 0.002917 | false | 0 / 0 |
| phase08-formal-20261029/S04/1/primary-E3 |   | correct | 报告主因为新版本 order-service 关闭 singleflight，使同商品缓存未命中时的并发请求无法合并、重复回源 MySQL，推高回源比例；与参考答案的 concurrent_cache_miss 机制一致，未提出相反结论。虽指出缺少缓存缺失触发原因和发布前配置，但机制方向正确。 | 0.500 | CNY 0.013452 | false | 0 / 0 |
| phase08-formal-20261029/S04/1/primary-E2 |   | correct | 报告将主因定位为商品缓存同键并发回源合并被新版本关闭（cache_singleflight=false），缓存未命中时并发查询分别回源放大数据库加载，与参考答案的 concurrent_cache_miss 机制一致，未提出相互冲突的主因，仅将初始未命中触发因素列为未确认。 | 0.667 | CNY 0.015807 | false | 0 / 0 |
| phase08-formal-20261029/S04/1/primary-E1 |   | correct | 报告定位 order-service:product_cache，认为新版本启动时关闭缓存 singleflight，导致同键并发未命中重复回源、放大数据库加载，与参考答案 concurrent_cache_miss 的关键因果一致，且未提出相反结论（如行锁集中）。 | 0.667 | CNY 0.013477 | false | 0 / 0 |
| phase08-formal-20261029/S04/2/comparison-E1 |  轮数达到上限 | partial | 报告确认了回源比例与回源速率大幅上升、同键并发回源合并未生效、流量增长不足以解释放大，方向与参考答案一致；但报告把主因归为客户端侧 Redis get 异常慢（0.8–1.5s）与新版本改动，未把"singleflight 被关闭"作为结论，且关键推断（版本引入 singleflight 关闭）在资料中缺少直接证据，属于找出部分原因但缺关键环节并引入冲突主因。 | 0.500 | CNY 0.027574 | false | 0 / 0 |
| phase08-formal-20261029/S04/2/primary-E1 |   | correct | 报告定位为商品缓存 singleflight 被关闭，导致同键并发查询不再合并、各自回源 MySQL，与参考答案 concurrent_cache_miss 的因果链一致，未误归因于库存行锁。 | 0.500 | CNY 0.012873 | false | 0 / 0 |
| phase08-formal-20261029/S04/2/primary-E3 |   | partial | 报告确认新版本关闭单一飞行合并，并观察到并发回源和未命中激增，与参考答案“同键并发未合并放大回源”部分吻合。但报告未验证“短时集中请求查同一商品”的热点批量场景，明确承认无商品维度和调用链证据，因果链缺少关键环节；且措辞“可能放大”属未证实推断，因此判为 partial。 | 0.667 | CNY 0.014964 | false | 0 / 0 |
| phase08-formal-20261029/S04/2/primary-E0 |   | incorrect | 报告只复述了 CacheLoadsHigh 告警（回源比例约78.7%持续30秒），并明确表示无法判断回源升高原因、组件为 unknown。参考答案的主因是缓存失效后同商品查询未合并导致的并发缓存未命中；报告未给出这一因果链，也未涉及 singleflight、商品查询分布或并发回源，仅将原因列为未知并建议人工核查，属于只有症状复述、主因缺失。 | 0.000 | CNY 0.003068 | false | 0 / 0 |
| phase08-formal-20261029/S04/2/primary-E2 |   | correct | 报告将主因定位为同时段重启导致 cache_singleflight 由 true 变为 false，同商品缓存未命中时并发请求无法合并、重复回源放大回源比例，与参考答案的 concurrent_cache_miss 机制一致，且未提出把下单集中到同一库存行等相反结论。 | 0.667 | CNY 0.015003 | false | 0 / 0 |
| phase08-formal-20261029/S04/3/comparison-E1 |  轮数达到上限 | correct | 报告主因与参考答案一致：热门商品短时并发查询、缓存未命中时同键回源未被 singleflight 合并，各自查 MySQL，导致回源比例上升；未把原因归于行锁或下单集中在同一库存行。并排除 Redis 淘汰、MySQL 锁等待等，仅有细节（并发计数、SQL 调用链）证据不足。 | 0.167 | CNY 0.020366 | false | 0 / 0 |
| phase08-formal-20261029/S04/3/primary-E1 |   | correct | 报告将主因定位为订单服务新版本关闭商品缓存 singleflight，导致同键并发缓存未命中无法合并、重复回源，符合参考答案的 concurrent_cache_miss 机制；报告也明确排除下单集中到同一库存行的说法（未提及行锁）。有配置日志、并发回源指标和升级时序支持。 | 0.500 | CNY 0.013240 | false | 0 / 0 |
| phase08-formal-20261029/S04/3/primary-E3 |   | correct | 报告主因是商品缓存同键并发回源未合并（cache_singleflight=false）导致重复访问数据库放大回源比例，与参考答案的并发缓存未命中机制一致；虽未确认首次失效原因，但关键因果链成立，未提出相反结论。 | 0.333 | CNY 0.012572 | false | 0 / 0 |
| phase08-formal-20261029/S04/3/primary-E2 |   | correct | 报告主因是发布版本将 cache_singleflight 从 true 改为 false，导致缓存未命中时同键并发请求无法合并、放大回源；与参考的并发缓存未命中机制核心一致，未提出相反结论。但部分里程碑（同键并发分布、行锁对照、SQL 调用链等）在报告中缺失或明确列为未知。 | 0.500 | CNY 0.014004 | false | 0 / 0 |
| phase08-formal-20261029/S04/3/primary-E0 |   | incorrect | 报告仅复述 CacheLoadsHigh 告警，明确将组件标为 unknown、机制未确定，并称信息不足无法区分缓存过期、请求分布变化或 Redis 异常；没有指出并发缓存未命中/同键查询未合并这一关键机制，也不存在相反结论之外的正确因果解释。仅有初始告警与报告提交步骤，无实际排查证据。 | 0.000 | CNY 0.003108 | false | 0 / 0 |
| phase08-formal-20261029/S05/1/primary-E1 |   | 评分失败 | 评分 JSON 格式不合法 | — | CNY 0.074693 | false | 0 / 0 |
| phase08-formal-20261029/S05/1/comparison-E1 |  轮数达到上限 | correct | 报告主因指向 order-service:product_cache 的 TTL 抖动被移除（1m0s→0s），商品键统一 TTL 后一批键集中到期、同时回源，形成回源洪峰，与参考答案 aligned_expiration 的因果关系一致，无相反结论；未直接读到逐键 TTL 分布这一点已作为 unknown 列出，不影响主因判断。 | 0.400 | CNY 0.020973 | false | 0 / 0 |
| phase08-formal-20261029/S05/1/primary-E0 |   | incorrect | 报告明确给出 component=unknown、confidence=0，只复述 CacheLoadsHigh 告警并列举无法区分缓存未命中、缓存访问异常或流量变化等原因，未提出“已存在的一批商品缓存共同到期、请求同时失去缓存保护导致集中回源”的主因。虽未明确排除缓存到期，但属于只有症状复述与不确定性声明，未解释关键因果关系，故机制判 incorrect。 | 0.000 | CNY 0.002863 | false | 0 / 0 |
| phase08-formal-20261029/S05/1/primary-E3 |   | correct | 报告主因与参考答案一致：新版本将商品缓存 TTL 抖动从 1m 改为 0，导致同批写入的缓存集中到期、请求集中未命中并回源升高。有配置变更（步骤25）和重启/版本变更（步骤8）作为支撑，未提出相反结论；虽未直接观测到具体缓存键集中到期，但机制方向正确。 | 0.400 | CNY 0.013022 | false | 0 / 0 |
| phase08-formal-20261029/S05/1/primary-E2 |   | 评分失败 | 评分 JSON 格式不合法 | — | CNY 0.020182 | false | 0 / 0 |
| phase08-formal-20261029/S05/2/primary-E3 |   | partial | 报告指出TTL抖动由1m变为0、回源与未命中比例升高，方向与「集中到期」一致，但明确承认未观察商品键到期分布、无调用链、无法确认唯一原因，关键因果环节（集中到期证据）缺失，故机制判partial。 | 0.200 | CNY 0.016358 | false | 0 / 0 |
| phase08-formal-20261029/S05/2/primary-E2 |   | partial | 报告主因指向 order-service 商品缓存 TTL 抖动由 1m 变为 0，使一批缓存集中到期引发未命中与回源，方向与参考 aligned_expiration 一致，且配置变更与告警时间相关性有据。但缺少关键环节：未取得缓存键到期时间分布的直接证据，实际回源比例查询峰值仅 28.3%，未能重现 35% 以上的告警窗口，因此集中到期这一核心因果未被观察证实，判 partial。 | 0.200 | CNY 0.016879 | false | 0 / 0 |
| phase08-formal-20261029/S05/2/primary-E1 |   | 评分失败 | 评分 JSON 格式不合法 | — | CNY 0.012992 | false | 0 / 0 |
| phase08-formal-20261029/S05/2/primary-E0 |   | incorrect | 报告只有初始告警，明确表示无法判断回源增加机制，把多种可能并列而未确定主因，未指向缓存集中到期导致同时失效这一机制。属于仅复述症状，不满足参考答案的因果关系。 | 0.000 | CNY 0.002847 | false | 0 / 0 |
| phase08-formal-20261029/S05/2/comparison-E1 |  轮数达到上限 | incorrect | 参考答案的主因是一批已存在的商品缓存共同到期（aligned_expiration），大量不同商品同时失去缓存保护而集中回源。报告的主因是发布/重启后冷启动导致进程内与本地缓存失效引起的回源突增，属于重启冷缓存，而非过期集中；报告虽提到“缓存为填充态”，但机制描述为启动失效，未识别 TTL 抖动/共同到期这一关键因果，且 Redis evicted_keys=0、144 万键仍在，说明并非键消失。机制与参考不符，仅有表面症状一致。 | 0.000 | CNY 0.022104 | false | 0 / 0 |
| phase08-formal-20261029/S05/3/primary-E3 |   | 评分失败 | 评分 JSON 格式不合法 | — | CNY 0.019675 | false | 0 / 0 |
| phase08-formal-20261029/S05/3/comparison-E1 |   | partial | 报告确实定位到 order-service:product_cache 的回源尖峰，并查到 cache_ttl_jitter 被改为 0s（步骤50）及重启（步骤11）。但其主因是“新版本将 TTL 抖动改为 0s 叠加重启清空缓存导致所有键获得同一固定 TTL、在过期时同时失效”——这是把原因归结为重启冷启动与未来批量过期风险，而非参考机制“已存在的一批商品缓存因共同到期而在同一时刻失去保护”。报告没有证据显示故障前缓存剩余时间集中或过期时刻同时回源，其自身证据（步骤22、25、33）也主要支持重启后冷启动回填。部分线索（TTL 抖动为 0、回源比例上升后回落）与参考机制相关，但关键因果环节（既有缓存共同到期）缺失，且主因表述与参考机制不一致，故判 partial。 | 0.200 | CNY 0.019043 | false | 0 / 0 |
| phase08-formal-20261029/S05/3/primary-E0 |   | incorrect | 报告只复述了告警阈值与回源比例，明确表示根因未知，未提出缓存集中到期导致同时回源的主因，反而仅把缓存过期列为待排查可能。机制与参考答案不符。 | 0.000 | CNY 0.002549 | false | 0 / 0 |
| phase08-formal-20261029/S05/3/primary-E2 |   | partial | 报告指出了 TTL 抖动为零可能造成商品键集中过期并引发回源增加，方向与参考机制一致，但明确指出“现有数据尚不足以证明这就是本次告警的原因”，未确认集中到期这一关键环节，属于部分命中。 | 0.400 | CNY 0.016565 | false | 0 / 0 |
| phase08-formal-20261029/S05/3/primary-E1 |   | partial | 报告指出 TTL 抖动由 1 分钟变为 0，并将其与重启后的回源峰值关联，方向与“已有缓存集中到期”部分吻合；但报告未取得缓存键过期时间分布、访问分布等直接证明集中到期的证据，且明确保留其他原因，关键因果链（过期前已有持续流量、回源比例>35% 持续 30 秒、回源调用链覆盖多个商品、DB 压力随重填下降）均未闭合，因此判为部分正确。 | 0.200 | CNY 0.015173 | false | 0 / 0 |
| phase08-formal-20261029/S06/1/primary-E2 |   | correct | 报告将主因定位为 order-worker 收到退出信号后关闭消费循环与管理入口，导致 Prometheus 采集失败和消息积压，与参考答案 stopped_consumer 机制一致，无相反结论；但缺少消费位点数据及恢复后追平、销量一致的验证。 | 0.500 | CNY 0.007107 | false | 0 / 0 |
| phase08-formal-20261029/S06/1/primary-E3 |   | correct | 报告将主因定位为 order-worker 停止（收到退出信号后结束消费循环与管理入口），与参考的 stopped_consumer 机制一致；日志、up 归零、管理端口告警与积压增长形成一致因果链，无相反结论。 | 0.750 | CNY 0.007222 | false | 0 / 0 |
| phase08-formal-20261029/S06/1/primary-E1 |   | correct | 报告主因与参考答案一致：order-worker 于 17:59:09 收到退出信号，消费循环与管理入口结束，导致位点不再提交、消息积压；未提出相反结论。 | 0.250 | CNY 0.006050 | false | 0 / 0 |
| phase08-formal-20261029/S06/1/comparison-E1 |   | correct | 报告明确主因为 order-worker 进程收到退出信号后优雅关停、停止消费，导致位点不提交、lag 增长、销量更新滞后，与 stopped_consumer 参考机制一致，未提出冲突主因。 | 0.750 | CNY 0.012939 | false | 0 / 0 |
| phase08-formal-20261029/S06/1/primary-E0 |   | incorrect | 报告只复述了管理端口采集失败告警，明确表示具体原因和业务影响无法确定，未指出消费者停止导致位点不提交、消息积压与销量落后的因果关系，主因缺失。 | 0.000 | CNY 0.002517 | false | 0 / 0 |
| phase08-formal-20261029/S06/2/primary-E2 |   | correct | 报告将主因定位为 order-worker 收到退出信号、结束消费循环与管理入口，导致管理端点不可采集与订单消息积压，与参考的停止消费者机制一致，未见相反结论。 | 0.250 | CNY 0.005913 | false | 0 / 0 |
| phase08-formal-20261029/S06/2/primary-E3 |   | correct | 报告主因为 order-worker 进程收到退出信号后消费循环与管理入口结束，导致采集失败和消费组积压，与参考的 stopped_consumer 机制一致，因果关系完整；未观察到未处理消息时销量落后的直接数据，但方向一致。 | 0.750 | CNY 0.006350 | false | 0 / 0 |
| phase08-formal-20261029/S06/2/comparison-E1 |   | correct | 报告主因是 order-worker 在 18:11:35 收到退出信号后停止消费循环与管理端口、此后未再启动，导致消息无人消费、lag 持续增长、销量滞后，与参考机制 stopped_consumer 一致，并排除流量突增与 order-service 故障，无明显相反结论。 | 0.750 | CNY 0.013707 | false | 0 / 0 |
| phase08-formal-20261029/S06/2/primary-E1 |   | correct | 报告主因与参考答案一致：order-worker 收到退出信号后停止消费循环，不再提交新位点，订单消息在该消费组持续积压，销量更新延迟；并有日志、up 指标、告警和 lag 指标支撑，未提出相反结论。 | 0.500 | CNY 0.006307 | false | 0 / 0 |
| phase08-formal-20261029/S06/2/primary-E0 |   | incorrect | 报告仅依据管理端口采集失败告警，明确表示无法区分进程、端口、网络或采集配置问题，不能认定 worker 停止或消费受影响；未给出参考答案的主因“消费者停止导致位点不提交、消息积压、销量落后”，也没有任何消费位点、lag 或订单写入的观测证据。 | 0.000 | CNY 0.002555 | false | 0 / 0 |
| phase08-formal-20261029/S06/3/primary-E1 |   | correct | 报告指出 order-worker 收到退出信号后结束消费循环和管理入口（进程停止），导致管理端口不可采集、消费位点不再提交、order.created 积压增长、异步销量更新延迟，与 stopped_consumer 机制一致，无相反结论。证据仅支持"收到退出信号"与"消费循环结束"，未无根据推断信号来源，属合理表述。 | 0.750 | CNY 0.007599 | false | 0 / 0 |
| phase08-formal-20261029/S06/3/primary-E0 |   | incorrect | 报告仅复述告警（管理端口无法采集）并明确表示无法确定根因，未识别出消费者停止导致位点不提交、消息积压这一机制，反而把主因列为 unknown，属于只有症状复述且未给出参考答案的主因。 | 0.000 | CNY 0.002507 | false | 0 / 0 |
| phase08-formal-20261029/S06/3/primary-E2 |   | correct | 报告明确 order-worker 收到退出信号后消费循环停止，管理端口不可采集、Kafka 积压增长、销量更新延迟，与参考答案 stopped_consumer 机制一致，上游退出原因标注为待查不等于否定主因。 | 0.500 | CNY 0.006247 | false | 0 / 0 |
| phase08-formal-20261029/S06/3/comparison-E1 |   | correct | 报告明确主因为 order-worker 消费者进程收到退出信号后停止、消费循环关闭，位点不再提交导致 order.created 积压增长，并通过日志、up=0、无 ERROR、MySQL 无锁等证据排除其他原因，与参考答案 stopped_consumer 机制一致。 | 0.750 | CNY 0.016258 | false | 0 / 0 |
| phase08-formal-20261029/S06/3/primary-E3 |   | correct | 报告主因为 order-worker 收到退出信号、消费循环与管理入口终止（消费者停止），这与参考的 stopped_consumer 机制一致，且用日志、up 指标、告警、Kafka lag 相互印证，无相反结论。 | 0.250 | CNY 0.006155 | false | 0 / 0 |
| phase08-formal-20261029/S07/1/primary-E1 |   | correct | 报告指出库存服务端毫秒级返回 OK、客户端 800ms 后 DEADLINE_EXCEEDED，定位到库存 RPC 响应路径/客户端处理环节，并提出扣减可能已生效但客户端未收到、商品页降级，与参考答案的网络等待型机制一致，无相反结论；未明确指出熔断打开，但主因判断正确。 | 0.500 | CNY 0.022407 | false | 0 / 0 |
| phase08-formal-20261029/S07/1/primary-E3 |   | correct | 报告定位为 order-service→inventory-service 的响应/收尾延迟：客户端 RPC 全部 DeadlineExceeded，而服务端为 OK 且耗时 1-5ms，两端速率一致，同一 trace 服务端成功但客户端 800ms 后才超时，符合“服务端处理完成后至客户端完成调用之间等待”的网络/链路延迟机制，并指出超时触发熔断、降级与扣减结果待核对，与参考答案一致。 | 1.000 | CNY 0.020870 | false | 0 / 0 |
| phase08-formal-20261029/S07/1/comparison-E1 |   | correct | 报告基于同一 trace 中客户端 span 800ms DEADLINE_EXCEEDED 而服务端仅 1–8ms 返回 OK、服务端无堆积、MySQL/Redis/Kafka 正常，判定故障在调用方与被调用方之间的传输/连接层，并未归因于库存业务处理。这与参考答案的 network_latency 机制一致，且无相反结论。 | 0.500 | CNY 0.018066 | false | 0 / 0 |
| phase08-formal-20261029/S07/1/primary-E0 |   | incorrect | 报告仅复述两条告警（HTTP 错误率升高、库存 RPC 失败率 100%），明确表示无法确定根因、组件为 unknown，未提出网络延迟导致调用方预算先耗尽的机制，也没有任何调用链/SQL/熔断证据，属只有症状复述且未给出主因。 | 0.000 | CNY 0.002747 | false | 0 / 0 |
| phase08-formal-20261029/S07/1/primary-E2 |   | correct | 报告定位 order-service→inventory-service 链路：客户端 span 800ms 超时、服务端 1~8ms 返回 OK，结论为返回路径/网络延迟类问题（机制指向 network_latency），与参考答案一致，无相反结论。虽未完全定位具体环节，但主因方向正确。 | 0.750 | CNY 0.014974 | false | 0 / 0 |
| phase08-formal-20261029/S07/2/primary-E2 |   | correct | 报告基于客户端 span 约 800ms 超时而服务端数毫秒返回 OK 的对比，定位到响应路径延迟/丢失（服务端处理完成后到客户端拿到结果之间），并说明库存查询因此降级、扣减超时导致待核对，与参考答案的网络延迟机制一致，无相反结论。 | 0.750 | CNY 0.016443 | false | 0 / 0 |
| phase08-formal-20261029/S07/2/primary-E1 |   | partial | 报告掌握了关键因果链的一半：库存服务端耗时仅数毫秒且返回 OK，而订单服务客户端 RPC 等到约 800ms 后 DEADLINE_EXCEEDED，即响应在网络/代理/客户端路径中滞留、调用方先用完预算；并据此提到商品库存降级与下单失败。但报告把根因表述为"服务端处理后响应路径或客户端处理异常，具体待验证"，未给出参考答案的网络延迟机制结论，且把"客户端处理异常"列为同等可能，主因存在相互冲突的表述，故判 partial。 | 0.500 | CNY 0.018136 | false | 0 / 0 |
| phase08-formal-20261029/S07/2/primary-E0 |   | incorrect | 报告未给出根因，component 为 unknown、confidence 为 0，只复述库存 RPC 失败比例告警并称信息不足，未识别网络延迟导致客户端先耗尽预算，也未涉及熔断打开或商品降级，主因缺失。 | 0.000 | CNY 0.002566 | false | 0 / 0 |
| phase08-formal-20261029/S07/2/primary-E3 |   | correct | 报告中给出主因：库存服务端处理很快（Deduct 约7ms、处理6ms，GetStock 约1ms），但订单侧客户端 RPC 在约800ms超时，说明响应在网络/返回路径上等待，与参考答案 network_latency 机制一致；并说明熔断打开放大不可用，未提出相反结论。 | 0.750 | CNY 0.021283 | false | 0 / 0 |
| phase08-formal-20261029/S07/2/comparison-E1 |  轮数达到上限 | 评分失败 | 评委必须且只能提交一次 submit_grade | — | CNY 0.018522 | false | 0 / 0 |
| phase08-formal-20261029/S07/3/primary-E2 |   | correct | 报告定位为客户端等待/响应返回链路异常：库存服务端快速成功（GetStock 1.42ms、Deduct 6ms），但 order-service 客户端 GetStock/Deduct 800ms DEADLINE_EXCEEDED，与参考答案 network_latency 机制一致，无相反结论。 | 1.000 | CNY 0.014085 | false | 0 / 0 |
| phase08-formal-20261029/S07/3/comparison-E1 |  轮数达到上限 | incorrect | 报告主因定位为 order-service 库存客户端的快速失败/状态判定错误，明确认为库存服务本身可用且“真实网络往返和库存服务处理都不像发生了”。这与参考答案的网络延迟机制相反：报告不但未能证明网络等待，反而用 8–11ms 快速失败否定了网络往返。虽正确识别失败发生在 order-service→inventory-service 调用路径上，但关键因果链（库存响应在网络中等待、调用方预算先耗尽、熔断打开）缺失，且矛盾点未解释。 | 0.000 | CNY 0.020292 | false | 0 / 0 |
| phase08-formal-20261029/S07/3/primary-E0 |   | incorrect | 报告仅复述两条告警并明确表示无法定位根因，未提出网络延迟导致库存调用超时、熔断打开及商品降级的机制，属于只有症状复述且未解释关键因果。 | 0.000 | CNY 0.002571 | false | 0 / 0 |
| phase08-formal-20261029/S07/3/primary-E3 |   | correct | 报告把主因定位为 order-service 到 inventory-service 的 RPC 路径/客户端接收环节异常：客户端约 800ms 报 DeadlineExceeded，服务端毫秒级 OK，属于网络/链路延迟性质，与参考答案 network_latency 一致，且有步骤 35、40、42 数据支撑；未错误归因到库存 SQL 或服务端。 | 0.750 | CNY 0.023501 | false | 0 / 0 |
| phase08-formal-20261029/S07/3/primary-E1 |   | correct | 报告基于步骤6/13/21的调用链证据指出：库存服务端 GetStock/Deduct 数毫秒返回 OK，但订单服务客户端约800ms后 DEADLINE_EXCEEDED，即响应在网络/客户端侧等待，属网络延迟方向，与参考机制一致；并说明库存查询降级、下单失败或待核对，未提出相反主因。 | 0.750 | CNY 0.015619 | false | 0 / 0 |
