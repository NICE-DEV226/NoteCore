<script lang="ts">
  /**
   * Identité : le symbole du logo (vectoriel, il prend la couleur du texte et
   * suit donc les deux thèmes) suivi du nom en toutes lettres. Le nom est du
   * texte, pas une image : net à toutes les tailles et lisible en sombre.
   */
  let { size = 30, stacked = false }: { size?: number; stacked?: boolean } = $props();
</script>

<span class="brand" class:stacked style:--size="{size}px">
  <svg class="mark" viewBox="160 171 211 229" role="img" aria-label="NoteCore">
    <g transform="translate(0,572) scale(0.1,-0.1)" fill="currentColor" stroke="none">
      <path d="M2195 3899 c-55 -33 -200 -116 -322 -186 l-223 -128 0 -716 0 -716
141 -114 c78 -63 176 -142 218 -176 l76 -62 30 18 c16 10 123 73 237 141 114
67 210 122 213 122 4 0 101 -70 217 -156 l210 -157 331 190 332 190 3 714 2
714 -217 174 c-120 95 -224 173 -233 173 -9 0 -118 -61 -242 -137 -124 -75
-230 -137 -236 -137 -7 0 -273 186 -405 284 l-32 23 -100 -58z m1015 -746 l-1
-708 -62 -36 c-34 -19 -99 -57 -143 -83 l-81 -47 -119 86 c-66 48 -120 90
-121 94 -5 13 266 173 279 165 7 -4 36 -24 63 -44 28 -20 67 -46 88 -59 l37
-23 0 421 0 420 -177 128 c-97 71 -178 133 -180 138 -3 9 392 255 409 255 4 0
8 -318 8 -707z m-760 241 c36 -25 89 -62 118 -81 28 -20 52 -39 52 -42 0 -3
-61 -43 -135 -88 l-134 -82 -101 69 -100 69 2 -422 3 -422 182 -136 183 -137
-158 -92 c-86 -51 -178 -106 -203 -121 -25 -16 -51 -29 -57 -29 -9 0 -12 175
-12 700 l0 700 27 17 c75 49 240 142 253 142 8 0 44 -20 80 -45z m325 -227
c50 -33 107 -73 128 -89 l37 -30 0 -177 c0 -157 -2 -180 -17 -193 -19 -16
-278 -177 -292 -181 -4 -1 -67 40 -139 92 l-132 94 0 181 0 181 146 90 c80 49
148 93 150 97 6 10 4 11 119 -65z" />
      <path d="M2520 2865 l0 -135 130 0 130 0 0 135 0 135 -130 0 -130 0 0 -135z" />
    </g>
  </svg>
  <span class="name" aria-hidden="true">Note<span class="core">Core</span></span>
</span>

<style>
  .brand {
    display: inline-flex;
    align-items: center;
    gap: calc(var(--size) * 0.36);
    color: var(--text);
  }
  .brand.stacked {
    flex-direction: column;
    gap: calc(var(--size) * 0.28);
  }
  /* Le symbole prend la couleur du texte : il suit les deux thèmes. */
  .mark {
    display: block;
    width: calc(var(--size) * 0.92);
    height: var(--size);
    flex-shrink: 0;
  }
  .name {
    font-family: var(--font-ui);
    font-size: calc(var(--size) * 0.64);
    font-weight: 750;
    letter-spacing: -0.035em;
    line-height: 1;
  }
  .core {
    color: var(--accent-text);
  }
</style>
